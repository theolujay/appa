package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func (app *application) serve() error {
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", app.config.port),
		Handler:      app.routes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		ErrorLog:     slog.NewLogLogger(app.logger.Handler(), slog.LevelError),
	}

	// Create a shutdownError channel to receive any errors returned by
	// the graceful Shutdown() function.
	shutdownError := make(chan error)

	// Start the background goroutine to run the app.hub.Run() method.
	// This will handle the registration, unregistration and broadcasting
	// of events to connected clients.
	app.background(app.hub.Run)

	// Start a background goroutine which will wait for an interrupt signal and
	// then gracefully shut down the server.
	go func() {
		// Create a 'quit' buffered channel which carries os.Signal values.
		// NOTE: a buffered channel is used here because signal.Notify() does
		// not wait for a receiver to be available when sending a signal to the
		// 'quit' channel. With an unbuffered channel, a signal could be 'missed'
		// if the 'quit' channl is not ready to recieve tat the exact moment the
		// sgnal is sent. A buffered channel avoids this problem and ensures we
		// never miss a signal.
		quit := make(chan os.Signal, 1)

		// Use signal.Notify() to listen for incoming SIGINT and SIGTERM signals
		// and relay them to the quit channel. Any other signals will not be
		// caught by signal.Notify() and will retain their default behavior.
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

		// Read the signal from the quit channel. This code will block until a
		// signal is received.
		s := <-quit

		// Log a message to say that the signal has been caught. Notice that
		// String() method is called on the signal to get the signal name and
		// include it in the log entry attributes.
		app.logger.Info("caught signal", "signal", s.String())

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Call Shutdown() on the server, passing in the context just created.
		// Shutdown() will return nil if the graceful shutdown was successful,
		// or an error (which may happen because of a problem closing the
		// listeners, or because the shutdown didn't complete before the
		// 30-second context deadline is hit). We relay this return value to
		// the shutdownError channel (if it returns an error).
		//
		// NOTE: Shutdown() method does not wait for any background tasks to
		// complete, nor does it close hijacked long-lived connections like
		// WebSockets. Instead, a custom logic to coordinate a
		// graceful shutdown has to be implemented.
		if err := srv.Shutdown(ctx); err != nil {
			shutdownError <- err
		}

		// Log a message to say that we're waiting for any background
		// goroutines to complete their tasks
		app.logger.Info("completing background tasks", "addr", srv.Addr)

		// Call Wait() to block until the WaitGroup counter is zero --
		// essentially blocking until the background goroutines have
		// finished. Then return nil on the shutdownError channel, to
		// indicate that the shutdown completed without any issues.
		app.wg.Wait()
		shutdownError <- nil
	}()

	app.logger.Info("starting server", "addr", srv.Addr, "env", app.config.env)

	// Calling Shutdown() on the server will cause ListenAndServe() to immediately
	// return http.ErrServerClosed error. This is a good thing, as it's an
	// indication that the graceful shutdown has started, so check specifically
	// for this, only returning the error if it is NOT http.ErrServerClosed.
	err := srv.ListenAndServe()
	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	// Otherwise, wait to receive the return value rom Shutdown() on the
	// shutdownError channel. If return value is an error, we know that there
	// was a problem with the graceful shutdown and we return the error.
	err = <-shutdownError
	if err != nil {
		return err
	}

	// At this point, we know that the graceful shutdown completed successfully
	// and we log a "stopped server" message.
	app.logger.Info("stopped server", "addr", srv.Addr)

	return nil
}
