package mailer

import (
	"bytes"
	"embed"
	"time"

	"github.com/wneessen/go-mail"

	ht "html/template"
	tt "text/template"
)

// The //go:embed directive bakes the templates directory into the binary
// at compile time. No template files need to exist on disk at runtime.
//
//go:embed "templates"
var templateFS embed.FS

type Mailer struct {
	client *mail.Client // Manages the SMTP connection.
	sender string       // From address: "Alice Smith <alice@example.com>"
}

// New configures and returns a Mailer. go-mail uses functional options
// (mail.WithPort, mail.WithUsername, etc.) to set fields on the client
// rather than a large config struct — a common Go pattern.
func New(host string, port int, username, password, sender string) (*Mailer, error) {
	client, err := mail.NewClient(
		host,
		mail.WithSMTPAuth(mail.SMTPAuthLogin),
		mail.WithPort(port),
		mail.WithUsername(username),
		mail.WithPassword(password),
		mail.WithTimeout(5*time.Second),
	)
	if err != nil {
		return nil, err
	}

	return &Mailer{client: client, sender: sender}, nil
}

// Send renders the named template file against data and delivers the
// result to recipient as a multipart email (plain text + HTML).
func (m *Mailer) Send(recipient, templateFile string, data any) error {
	// Parse the template file once with text/template. A single file
	// contains multiple named blocks (subject, plainBody, htmlBody) —
	// ParseFS registers all of them from the embedded filesystem.
	textTmpl, err := tt.New("").ParseFS(templateFS, "templates/"+templateFile)
	if err != nil {
		return err
	}

	// ExecuteTemplate renders one named block into the buffer.
	// bytes.Buffer satisfies io.Writer (required by ExecuteTemplate)
	// and lets us read the result back as a string with .String().
	subject := new(bytes.Buffer)
	if err = textTmpl.ExecuteTemplate(subject, "subject", data); err != nil {
		return err
	}

	plainBody := new(bytes.Buffer)
	if err = textTmpl.ExecuteTemplate(plainBody, "plainBody", data); err != nil {
		return err
	}

	// Re-parse with html/template for the HTML body. html/template's
	// auto-escaping prevents XSS — the same template file, different engine.
	htmlTmpl, err := ht.New("").ParseFS(templateFS, "templates/"+templateFile)
	if err != nil {
		return nil
	}

	htmlBody := new(bytes.Buffer)
	if err = htmlTmpl.ExecuteTemplate(htmlBody, "htmlBody", data); err != nil {
		return err
	}

	// Assemble the MIME message. SetBodyString sets the primary body;
	// AddAlternativeString attaches the HTML version as a fallback.
	// The recipient's email client picks whichever it can render.
	msg := mail.NewMsg()
	if err = msg.To(recipient); err != nil {
		return err
	}
	if err = msg.From(m.sender); err != nil {
		return err
	}

	msg.Subject(subject.String())
	msg.SetBodyString(mail.TypeTextPlain, plainBody.String())
	msg.AddAlternativeString(mail.TypeTextHTML, htmlBody.String())

	// Try sending the email up to three times before aborting and
	// returning the final error. Sleep for 500 milliseconds between
	// each attempt.
	for i := 1; i <= 3; i++ {
		// Open the SMTP connection, deliver the message, then close
		// the connection.
		err = m.client.DialAndSend(msg)
		if err == nil {
			return nil
		}
		if i != 3 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	return err
}
