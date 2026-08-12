package mailer

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/inbucket/html2text"
	"gopkg.in/gomail.v2"
)

const HTMLMIMEType = "text/html"
const TextMIMEType = "text/plain"

// smtpPort is the port the SMTP relay listens on. local-exim, the only relay this service
// talks to, listens on the standard port.
const smtpPort = 25

// smtpLocalName is the name sent in the SMTP HELO. The relay authorizes senders by source
// address rather than by HELO name, so this only has to identify the sender in mail logs.
const smtpLocalName = "notifications"

// FormattedEmailRequest represents a request to send an email that has already been formatted.
type FormattedEmailRequest struct {
	To          []string
	Cc          []string
	Bcc         []string
	From        string
	MIMEType    string
	Subject     string
	Body        string
	Attachments []Attachment
}

// Attachment is a file attached to an outgoing email.
type Attachment struct {
	Filename string
	Data     string // Base64-encoded file data
}

// Validate returns an error if the email request is invalid.
func (r *FormattedEmailRequest) Validate() error {
	if len(r.To) == 0 {
		return fmt.Errorf("at least one destination email address must be provided")
	}
	if r.Subject == "" {
		return fmt.Errorf("a message subject must be provided")
	}
	if r.Body == "" {
		return fmt.Errorf("a message body must be provided")
	}
	return nil
}

// EmailClient is a client used to send email messages to an SMTP server.
type EmailClient struct {
	smtpHost    string
	smtpPort    int
	fromAddress string
}

// NewEmailClient creates a new email client.
func NewEmailClient(smtpHost string, from string) *EmailClient {
	return &EmailClient{
		smtpHost:    smtpHost,
		smtpPort:    smtpPort,
		fromAddress: from,
	}
}

// GetFromAddress returns the source email address. If the source address is provided in the
// email request then that email address is used. Otherwise, the default address configured in
// the email client is used.
func (r *EmailClient) GetFromAddress(req *FormattedEmailRequest) string {
	fromAddress := req.From
	if fromAddress == "" {
		fromAddress = r.fromAddress
	}
	return fromAddress
}

// Send sends an email.
func (r *EmailClient) Send(ctx context.Context, req *FormattedEmailRequest) error {
	log := log.WithContext(ctx)

	if err := req.Validate(); err != nil {
		log.Errorf("invalid email request: %s", err)
		return err
	}

	m := gomail.NewMessage()
	m.SetHeader("From", r.GetFromAddress(req))
	m.SetHeader("mailed-by", "cyverse.org")
	m.SetHeader("To", req.To...)
	if len(req.Cc) != 0 {
		m.SetHeader("Cc", req.Cc...)
	}
	if len(req.Bcc) != 0 {
		m.SetHeader("Bcc", req.Bcc...)
	}
	m.SetHeader("Subject", req.Subject)

	for _, attachment := range req.Attachments {
		decodedData, err := base64.StdEncoding.DecodeString(attachment.Data)
		if err != nil {
			log.Errorf("failed to decode attachment %s: %s", attachment.Filename, err)
			continue
		}
		m.Attach(attachment.Filename, gomail.SetCopyFunc(func(w io.Writer) error {
			_, err := w.Write(decodedData)
			return err
		}))
	}

	// HTML messages go out as multipart with a generated plain-text alternative, so clients
	// that won't render HTML still get a readable body.
	if req.MIMEType == HTMLMIMEType {
		plaintext, err := html2text.FromString(req.Body)
		if err != nil {
			m.SetBody(req.MIMEType, req.Body)
			log.Info(err)
		} else {
			m.SetBody(TextMIMEType, plaintext)
			m.AddAlternative(req.MIMEType, req.Body)
		}
	} else {
		m.SetBody(req.MIMEType, req.Body)
	}

	d := gomail.Dialer{Host: r.smtpHost, Port: r.smtpPort, LocalName: smtpLocalName}
	if err := d.DialAndSend(m); err != nil {
		log.Error(err)
		return err
	}

	return nil
}
