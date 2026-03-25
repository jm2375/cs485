package services

import (
	"fmt"
	"log"
)

// IEmailService is the interface for sending transactional emails.
// The real SendGrid implementation is swapped in for P5.
type IEmailService interface {
	SendInviteEmail(to, inviterName, tripName, inviteLink string) error
}

// MockEmailService logs email sends to stdout instead of making network calls.
// It satisfies IEmailService for P4.
type MockEmailService struct{}

func NewMockEmailService() *MockEmailService { return &MockEmailService{} }

func (m *MockEmailService) SendInviteEmail(to, inviterName, tripName, inviteLink string) error {
	log.Println("─────────────────────────────────────────────")
	log.Printf("[email] TO:      %s", to)
	log.Printf("[email] FROM:    %s (via TripPlanner)", inviterName)
	log.Printf("[email] SUBJECT: You've been invited to join \"%s\"", tripName)
	log.Printf("[email] BODY:    %s has invited you to collaborate on \"%s\".", inviterName, tripName)
	log.Printf("[email]          Accept your invitation: %s", inviteLink)
	log.Println("─────────────────────────────────────────────")
	fmt.Printf("\n[MockEmail] Invite sent to %s — link: %s\n\n", to, inviteLink)
	return nil
}
