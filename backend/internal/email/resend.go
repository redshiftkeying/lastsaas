package email

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"text/template"

	"lastsaas/internal/apicounter"
)

type ResendService struct {
	apiKey      string
	fromEmail   string
	fromName    string
	appName     string
	baseURL     string
	frontendURL string
	getConfig   func(string) string
}

type emailRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
}

func NewResendService(apiKey, fromEmail, fromName, appName, frontendURL string, getConfig func(string) string) *ResendService {
	return &ResendService{
		apiKey:      apiKey,
		fromEmail:   fromEmail,
		fromName:    fromName,
		appName:     appName,
		baseURL:     "https://api.resend.com",
		frontendURL: frontendURL,
		getConfig:   getConfig,
	}
}

func (s *ResendService) from() string {
	return fmt.Sprintf("%s <%s>", s.fromName, s.fromEmail)
}

func (s *ResendService) SendEmail(to, subject, html string) error {
	reqBody := emailRequest{
		From:    s.from(),
		To:      []string{to},
		Subject: subject,
		HTML:    html,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal email request: %w", err)
	}

	req, err := http.NewRequest("POST", s.baseURL+"/emails", bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Email API error: status %d, body: %s", resp.StatusCode, string(body))
		return fmt.Errorf("email API returned status %d", resp.StatusCode)
	}

	apicounter.ResendEmails.Add(1)
	log.Printf("Email sent successfully to %s", to)
	return nil
}

// resolveAppName returns the app name from the config system, falling back to the constructor value.
func (s *ResendService) resolveAppName() string {
	if s.getConfig != nil {
		if name := s.getConfig("app.name"); name != "" {
			return name
		}
	}
	return s.appName
}

// executeTemplate loads a template from config, parses it, and executes with the given data.
// If the config value is empty or parsing/execution fails, the fallback string is returned.
func (s *ResendService) executeTemplate(configKey string, data map[string]string, fallback string) string {
	tmplStr := ""
	if s.getConfig != nil {
		tmplStr = s.getConfig(configKey)
	}
	if tmplStr == "" {
		tmplStr = fallback
	}

	t, err := template.New(configKey).Parse(tmplStr)
	if err != nil {
		log.Printf("email: failed to parse template %s, using fallback: %v", configKey, err)
		return fallback
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		log.Printf("email: failed to execute template %s, using fallback: %v", configKey, err)
		return fallback
	}
	return buf.String()
}

func (s *ResendService) SendVerificationEmail(to, displayName, token string) error {
	verifyURL := fmt.Sprintf("%s/verify-email?token=%s", s.frontendURL, token)
	appName := s.resolveAppName()

	data := map[string]string{
		"AppName":     appName,
		"DisplayName": displayName,
		"VerifyURL":   verifyURL,
	}

	subject := s.executeTemplate("email.verification.subject", data,
		fmt.Sprintf("Verify your %s account", appName))

	fallbackBody := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background-color: #f8fafc; color: #1e293b; margin: 0; padding: 40px 20px;">
    <div style="max-width: 600px; margin: 0 auto; background: #ffffff; border: 1px solid #e2e8f0; border-radius: 12px; padding: 40px;">
        <h1 style="color: #0f172a; margin: 0 0 8px 0; font-size: 24px;">%s</h1>
        <hr style="border: none; border-top: 1px solid #e2e8f0; margin: 20px 0;">
        <h2 style="color: #1e293b; margin-bottom: 16px;">Verify Your Email</h2>
        <p style="color: #475569; line-height: 1.6;">Hi %s,</p>
        <p style="color: #475569; line-height: 1.6;">Thanks for signing up! Please verify your email address by clicking the button below:</p>
        <div style="text-align: center; margin: 30px 0;">
            <a href="%s" style="display: inline-block; background: #2563eb; color: white; text-decoration: none; padding: 14px 32px; border-radius: 8px; font-weight: 600; font-size: 16px;">Verify Email Address</a>
        </div>
        <p style="color: #94a3b8; font-size: 14px;">If you didn't create an account, you can safely ignore this email.</p>
        <p style="color: #94a3b8; font-size: 14px;">This link will expire in 24 hours.</p>
    </div>
</body>
</html>`, appName, displayName, verifyURL)

	html := s.executeTemplate("email.verification.body", data, fallbackBody)

	return s.SendEmail(to, subject, html)
}

func (s *ResendService) SendPasswordResetEmail(to, displayName, token string) error {
	resetURL := fmt.Sprintf("%s/reset-password?token=%s", s.frontendURL, token)
	appName := s.resolveAppName()

	data := map[string]string{
		"AppName":     appName,
		"DisplayName": displayName,
		"ResetURL":    resetURL,
	}

	subject := s.executeTemplate("email.password_reset.subject", data,
		fmt.Sprintf("Reset your %s password", appName))

	fallbackBody := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background-color: #f8fafc; color: #1e293b; margin: 0; padding: 40px 20px;">
    <div style="max-width: 600px; margin: 0 auto; background: #ffffff; border: 1px solid #e2e8f0; border-radius: 12px; padding: 40px;">
        <h1 style="color: #0f172a; margin: 0 0 8px 0; font-size: 24px;">%s</h1>
        <hr style="border: none; border-top: 1px solid #e2e8f0; margin: 20px 0;">
        <h2 style="color: #1e293b; margin-bottom: 16px;">Reset Your Password</h2>
        <p style="color: #475569; line-height: 1.6;">Hi %s,</p>
        <p style="color: #475569; line-height: 1.6;">We received a request to reset your password. Click the button below to choose a new password:</p>
        <div style="text-align: center; margin: 30px 0;">
            <a href="%s" style="display: inline-block; background: #2563eb; color: white; text-decoration: none; padding: 14px 32px; border-radius: 8px; font-weight: 600; font-size: 16px;">Reset Password</a>
        </div>
        <p style="color: #94a3b8; font-size: 14px;">If you didn't request a password reset, you can safely ignore this email.</p>
        <p style="color: #94a3b8; font-size: 14px;">This link will expire in 1 hour.</p>
    </div>
</body>
</html>`, appName, displayName, resetURL)

	html := s.executeTemplate("email.password_reset.body", data, fallbackBody)

	return s.SendEmail(to, subject, html)
}

func (s *ResendService) SendInvitationEmail(to, inviterName, tenantName, token string) error {
	inviteURL := fmt.Sprintf("%s/signup?invitation=%s", s.frontendURL, token)
	appName := s.resolveAppName()

	data := map[string]string{
		"AppName":     appName,
		"InviterName": inviterName,
		"TenantName":  tenantName,
		"InviteURL":   inviteURL,
	}

	subject := s.executeTemplate("email.invitation.subject", data,
		fmt.Sprintf("You've been invited to %s", tenantName))

	fallbackBody := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background-color: #f8fafc; color: #1e293b; margin: 0; padding: 40px 20px;">
    <div style="max-width: 600px; margin: 0 auto; background: #ffffff; border: 1px solid #e2e8f0; border-radius: 12px; padding: 40px;">
        <h1 style="color: #0f172a; margin: 0 0 8px 0; font-size: 24px;">%s</h1>
        <hr style="border: none; border-top: 1px solid #e2e8f0; margin: 20px 0;">
        <h2 style="color: #1e293b; margin-bottom: 16px;">You've Been Invited</h2>
        <p style="color: #475569; line-height: 1.6;">%s has invited you to join <strong>%s</strong>.</p>
        <div style="text-align: center; margin: 30px 0;">
            <a href="%s" style="display: inline-block; background: #2563eb; color: white; text-decoration: none; padding: 14px 32px; border-radius: 8px; font-weight: 600; font-size: 16px;">Accept Invitation</a>
        </div>
        <p style="color: #94a3b8; font-size: 14px;">This invitation will expire in 7 days.</p>
    </div>
</body>
</html>`, appName, inviterName, tenantName, inviteURL)

	html := s.executeTemplate("email.invitation.body", data, fallbackBody)

	return s.SendEmail(to, subject, html)
}
