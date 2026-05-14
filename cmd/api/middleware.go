package main

import (
	"bufio"
	"errors"
	"expvar"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/tomasen/realip"
	"golang.org/x/time/rate"
)

func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(
			"Content-Security-Policy",
			"default-src 'self'; style-src 'self' fonts.googleapis.com; font-src fonts.gstatic.com",
		)
		w.Header().Set("Referre-Policy", "origin-when-cross-origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "deny")
		w.Header().Set("X-XSS-Protection", "0")
		next.ServeHTTP(w, r)
	})
}

func (app *application) logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		app.logger.Info(
			"%s - %s %s %s", r.RemoteAddr, r.Proto, r.Method, r.URL.RequestURI(),
		)
		next.ServeHTTP(w, r)
	})
}

// The recoverPanic() method is a middleware for the server to send a
// 500 Internal Server Error when it panics rather than just closing
// the HTTP connection with no context. It doesn't recover panics in
// other goroutines, so ensure to recover panics from within those
// goroutines.
//
// NOTE:
//
// http.Server may still automatically generate and send plain-text
// HTTP responses in the following scenarios:
//
// - The HTTP request specifies an unsupported HTTP protocol version.
//
// - The HTTP request contains a missing or invalid Host header, or multiple Host headers.
//
// - The HTTP request contains a empty Content-Length header.
//
// - The HTTP request contains an unsupported Transfer-Encoding header.
//
// - The size of the HTTP request headers exceeds the server’s MaxHeaderBytes setting.
//
// - The client makes a HTTP request to a HTTPS server.
func (app *application) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				w.Header().Set("Connection", "close")
				app.serverErrorResponse(w, r, fmt.Errorf("%s", err))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// The rateLimit() method is a middleware that implements rate-limiter
// based on the IP address of the client.
func (app *application) rateLimit(next http.Handler) http.Handler {
	// Hold the rate limiter and last seen time for each client.
	type client struct {
		limiter  *rate.Limiter
		lastSeen time.Time
	}
	// A mutex and a map to hold the clients' IP addresses
	// and rate limiters.
	var (
		mu      sync.Mutex
		clients = make(map[string]*client)
	)

	// Launch a background goroutine which removes old entries from
	// the clients map once every minute.
	go func() {
		for {
			time.Sleep(time.Minute)

			// Lock the mutex to prevent any rate limiter checks from
			// happening while the cleanup is taking place.
			mu.Lock()

			// Loop through all clients. If they haven't been seen
			// within the last three minutes, delete the corresponding
			// entry from the map.
			for ip, client := range clients {
				if time.Since(client.lastSeen) > 3*time.Minute {
					delete(clients, ip)
				}
			}
			// Unlock the mutext when the clean up is complete
			mu.Unlock()
		}
	}()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if app.config.limiter.enabled {
			// Get client's IP address
			ip := realip.FromRequest(r)

			mu.Lock()

			// Check to see if the IP address already exists in the map. If
			// it doesn't , then initialize the new rate limiter and add the
			// IP address and limier to the map.
			if _, found := clients[ip]; !found {
				clients[ip] = &client{
					limiter: rate.NewLimiter(
						rate.Limit(app.config.limiter.rps),
						app.config.limiter.burst,
					),
				}
			}

			// Update the last seen time for the client
			clients[ip].lastSeen = time.Now()

			// Call the Allow() method on the rate limiter for the current
			// IP address. If the request isn't allowed, unlock the mutex
			// and send a 429 Too Many Requests response.
			if !clients[ip].limiter.Allow() {
				mu.Unlock()
				app.rateLimitExceededResponse(w, r)
				return
			}

			// Make sure to unlock the mutex before calling the next handler
			// in the chain. And notice how `defer` isn't used to unlock the
			// mutext, as that would mean that the mutext isn't unlocked
			// until all the handlers downstream of this middleware have
			// also returned.
			mu.Unlock()
		}

		next.ServeHTTP(w, r)
	})
}

func (app *application) enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Origin")
		w.Header().Add("Vary", "Access-Control-Request-Method")

		origin := r.Header.Get("Origin")

		if origin != "" && slices.Contains(app.config.cors.trustedOrigins, origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)

			// Check if the request has the HTTP method OPTIONS and contains the
			// "Access-Control-Request-Method" header. If it does, then treat it
			// as a preflight request.
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				// Set the necessary preflisht response headers
				w.Header().Set("Access-Control-Allow-Methods", "OPTIONS, PUT, PATCH, DELETE")
				// Since "Authorization" header is allowed, it's important to
				// not set the wild card "Access-Control-Allow-Origin: *" header
				// without checking against a list of trusted origins, as done
				// above.
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")

				// Write the headers along with a 200 OK and return from the
				// the middleware with no further action. 200 OK is preferred
				// to 204 No Content, as not a certain browser versions may
				// not support 204 No Content responses and subsequently block
				// the real request.
				w.WriteHeader(http.StatusOK)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func (app *application) metrics(next http.Handler) http.Handler {
	var (
		totalRequestsReceived           = expvar.NewInt("total_requests_received")
		totalResponsesSent              = expvar.NewInt("total_resposes_sent")
		totalProcessingTimeMicroseconds = expvar.NewInt("total_processing_time_μs")
		totalResponsesSentByStatus      = expvar.NewMap("total_repsonses_sent_by_status")
	)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Record the time that the we started to process the request
		start := time.Now()
		// Increment the number of requests received by 1
		totalRequestsReceived.Add(1)
		// Wrap the original http.ResponseWriter value the metrics
		// received in a new metricsResponseWriter
		mw := newMetricsResponseWriter(w)
		// Call the next handler in the chain using the new
		// metricsResponseWriter sa the http.ResponseWriter value
		next.ServeHTTP(mw, r)
		// On the way back up the middleware chain, increment
		// the number of responses
		totalResponsesSent.Add(1)
		// The response status code should be stored in the mw.statusCode
		// field by now. NOTE: the expvar map is string-keyed, so the
		// status code (which is an integer) is converted to a string using
		// the strconv.Itoa() function.
		totalResponsesSentByStatus.Add(strconv.Itoa(mw.statusCode), 1)
		// Calculate the number of microseconds since we began to
		// process the request, then increment the total processing
		// time by this amount
		duration := time.Since(start).Microseconds()

		totalProcessingTimeMicroseconds.Add(duration)
	})
}

// The metricsResponseWriter type wraps an existing http.ResponseWriter
// and also contains a field for recording the response status code, and
// a boolean flag to indicate whether the reponse deaders have already
// been written.
type metricsResponseWriter struct {
	wrapped       http.ResponseWriter
	statusCode    int
	headerWritten bool
}

// newMetricsResponseWriter() returns a new metricsResponseWriter instance
// which wraps a given http.ResponseWriter and has a status code of 200
// (which is the status code that Go will send in a HTTP repsonse by default).
func newMetricsResponseWriter(w http.ResponseWriter) *metricsResponseWriter {
	return &metricsResponseWriter{
		wrapped:    w,
		statusCode: http.StatusOK,
	}
}

// The Header() method is a simple 'pass-through' to the Header() method
// of the wrapped http.ResponseWriter.
func (mw *metricsResponseWriter) Header() http.Header {
	return mw.wrapped.Header()
}

// The WriteHeader() is a pass-through to the WriteHeader() method
// of the wrapped http.ResponseWriter. But after this returns, we
// also record the response status code (it it hasn't already been
// recorded) and set the headerWritten field to true to indicate
// that the HTTP response headers hve now been written.
func (mw *metricsResponseWriter) WriteHeader(statusCode int) {
	mw.wrapped.WriteHeader(statusCode)

	if !mw.headerWritten {
		mw.statusCode = statusCode
		mw.headerWritten = true
	}
}

// The Write() method does a 'pass-through' to the Write() method
// of the wrapped http.ResponseWriter. Calling this will automatically
// write any response headers, so we set the headerWritten field to true.
func (mw *metricsResponseWriter) Write(b []byte) (int, error) {
	mw.headerWritten = true
	return mw.wrapped.Write(b)
}

// The Unwrap() method returns the existing wrapped http.ResponseWriter
func (mw *metricsResponseWriter) Unwrap() http.ResponseWriter {
	return mw.wrapped
}

// Hijack implements http.Hijacker. It simply wraps the underlying
// ResponseWriter's Hijack method if there's one, or returns an error.
func (mw *metricsResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := mw.wrapped.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, errors.New("not a Hijacker")
}
