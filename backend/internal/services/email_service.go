package services

import (
	"fmt"

	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

// IEmailService is the interface for sending transactional emails.
type IEmailService interface {
	SendInviteEmail(to, inviterName, tripName, inviteLink string) error
}

// SendGridEmailService sends transactional emails via the SendGrid API.
type SendGridEmailService struct {
	apiKey     string
	fromEmail  string
}

func NewSendGridEmailService(apiKey string) *SendGridEmailService {
	return &SendGridEmailService{
		apiKey:    apiKey,
		fromEmail: "at859@njit.edu",
	}
}

func (s *SendGridEmailService) SendInviteEmail(to, inviterName, tripName, inviteLink string) error {
	from := mail.NewEmail(fmt.Sprintf("%s (via TripPlanner)", inviterName), s.fromEmail)
	toAddr := mail.NewEmail("", to)
	subject := fmt.Sprintf("You've been invited to join \"%s\"", tripName)
	plainText := fmt.Sprintf(
		"%s has invited you to collaborate on \"%s\".\nAccept your invitation: %s",
		inviterName, tripName, inviteLink,
	)
	htmlContent := fmt.Sprintf(
		`<p>%s has invited you to collaborate on <strong>"%s"</strong>.</p><p><a href="%s">Accept your invitation</a></p>`,
		inviterName, tripName, inviteLink,
	)

	message := mail.NewSingleEmail(from, subject, toAddr, plainText, htmlContent)
	client := sendgrid.NewSendClient(s.apiKey)
	response, err := client.Send(message)
	if err != nil {
		return fmt.Errorf("sendgrid: %w", err)
	}
	if response.StatusCode >= 400 {
		return fmt.Errorf("sendgrid: unexpected status %d: %s", response.StatusCode, response.Body)
	}
	return nil
}
