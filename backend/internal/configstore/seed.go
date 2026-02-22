package configstore

import (
	"context"
	"log"
	"time"

	"lastsaas/internal/db"
	"lastsaas/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// SystemDefaults defines the system-level configuration variables that must always exist.
var SystemDefaults = []models.ConfigVar{
	{
		Name:        "app.name",
		Description: "Application name used in email templates and other system text (referenced as {{.AppName}} in templates)",
		Type:        models.ConfigTypeString,
		Value:       "LastSaaS",
		IsSystem:    true,
	},
	{
		Name:        "email.verification.subject",
		Description: "Subject line for the email verification message",
		Type:        models.ConfigTypeTemplate,
		Value:       `Verify your {{.AppName}} account`,
		IsSystem:    true,
	},
	{
		Name:        "email.verification.body",
		Description: "HTML body for the email verification message",
		Type:        models.ConfigTypeTemplate,
		Value: `<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background-color: #f8fafc; color: #1e293b; margin: 0; padding: 40px 20px;">
    <div style="max-width: 600px; margin: 0 auto; background: #ffffff; border: 1px solid #e2e8f0; border-radius: 12px; padding: 40px;">
        <h1 style="color: #0f172a; margin: 0 0 8px 0; font-size: 24px;">{{.AppName}}</h1>
        <hr style="border: none; border-top: 1px solid #e2e8f0; margin: 20px 0;">
        <h2 style="color: #1e293b; margin-bottom: 16px;">Verify Your Email</h2>
        <p style="color: #475569; line-height: 1.6;">Hi {{.DisplayName}},</p>
        <p style="color: #475569; line-height: 1.6;">Thanks for signing up! Please verify your email address by clicking the button below:</p>
        <div style="text-align: center; margin: 30px 0;">
            <a href="{{.VerifyURL}}" style="display: inline-block; background: #2563eb; color: white; text-decoration: none; padding: 14px 32px; border-radius: 8px; font-weight: 600; font-size: 16px;">Verify Email Address</a>
        </div>
        <p style="color: #94a3b8; font-size: 14px;">If you didn't create an account, you can safely ignore this email.</p>
        <p style="color: #94a3b8; font-size: 14px;">This link will expire in 24 hours.</p>
    </div>
</body>
</html>`,
		IsSystem: true,
	},
	{
		Name:        "email.password_reset.subject",
		Description: "Subject line for the password reset email",
		Type:        models.ConfigTypeTemplate,
		Value:       `Reset your {{.AppName}} password`,
		IsSystem:    true,
	},
	{
		Name:        "email.password_reset.body",
		Description: "HTML body for the password reset email",
		Type:        models.ConfigTypeTemplate,
		Value: `<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background-color: #f8fafc; color: #1e293b; margin: 0; padding: 40px 20px;">
    <div style="max-width: 600px; margin: 0 auto; background: #ffffff; border: 1px solid #e2e8f0; border-radius: 12px; padding: 40px;">
        <h1 style="color: #0f172a; margin: 0 0 8px 0; font-size: 24px;">{{.AppName}}</h1>
        <hr style="border: none; border-top: 1px solid #e2e8f0; margin: 20px 0;">
        <h2 style="color: #1e293b; margin-bottom: 16px;">Reset Your Password</h2>
        <p style="color: #475569; line-height: 1.6;">Hi {{.DisplayName}},</p>
        <p style="color: #475569; line-height: 1.6;">We received a request to reset your password. Click the button below to choose a new password:</p>
        <div style="text-align: center; margin: 30px 0;">
            <a href="{{.ResetURL}}" style="display: inline-block; background: #2563eb; color: white; text-decoration: none; padding: 14px 32px; border-radius: 8px; font-weight: 600; font-size: 16px;">Reset Password</a>
        </div>
        <p style="color: #94a3b8; font-size: 14px;">If you didn't request a password reset, you can safely ignore this email.</p>
        <p style="color: #94a3b8; font-size: 14px;">This link will expire in 1 hour.</p>
    </div>
</body>
</html>`,
		IsSystem: true,
	},
	{
		Name:        "email.invitation.subject",
		Description: "Subject line for the team invitation email",
		Type:        models.ConfigTypeTemplate,
		Value:       `You've been invited to {{.TenantName}}`,
		IsSystem:    true,
	},
	{
		Name:        "email.invitation.body",
		Description: "HTML body for the team invitation email",
		Type:        models.ConfigTypeTemplate,
		Value: `<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background-color: #f8fafc; color: #1e293b; margin: 0; padding: 40px 20px;">
    <div style="max-width: 600px; margin: 0 auto; background: #ffffff; border: 1px solid #e2e8f0; border-radius: 12px; padding: 40px;">
        <h1 style="color: #0f172a; margin: 0 0 8px 0; font-size: 24px;">{{.AppName}}</h1>
        <hr style="border: none; border-top: 1px solid #e2e8f0; margin: 20px 0;">
        <h2 style="color: #1e293b; margin-bottom: 16px;">You've Been Invited</h2>
        <p style="color: #475569; line-height: 1.6;">{{.InviterName}} has invited you to join <strong>{{.TenantName}}</strong>.</p>
        <div style="text-align: center; margin: 30px 0;">
            <a href="{{.InviteURL}}" style="display: inline-block; background: #2563eb; color: white; text-decoration: none; padding: 14px 32px; border-radius: 8px; font-weight: 600; font-size: 16px;">Accept Invitation</a>
        </div>
        <p style="color: #94a3b8; font-size: 14px;">This invitation will expire in 7 days.</p>
    </div>
</body>
</html>`,
		IsSystem: true,
	},
	{
		Name:        "health.cpu.warning_threshold",
		Description: "CPU usage percentage that triggers a warning status indicator",
		Type:        models.ConfigTypeNumeric,
		Value:       "70",
		IsSystem:    true,
	},
	{
		Name:        "health.cpu.critical_threshold",
		Description: "CPU usage percentage that triggers a critical status indicator",
		Type:        models.ConfigTypeNumeric,
		Value:       "90",
		IsSystem:    true,
	},
	{
		Name:        "health.memory.warning_threshold",
		Description: "Memory usage percentage that triggers a warning status indicator",
		Type:        models.ConfigTypeNumeric,
		Value:       "75",
		IsSystem:    true,
	},
	{
		Name:        "health.memory.critical_threshold",
		Description: "Memory usage percentage that triggers a critical status indicator",
		Type:        models.ConfigTypeNumeric,
		Value:       "90",
		IsSystem:    true,
	},
	{
		Name:        "health.disk.warning_threshold",
		Description: "Disk usage percentage that triggers a warning status indicator",
		Type:        models.ConfigTypeNumeric,
		Value:       "80",
		IsSystem:    true,
	},
	{
		Name:        "health.disk.critical_threshold",
		Description: "Disk usage percentage that triggers a critical status indicator",
		Type:        models.ConfigTypeNumeric,
		Value:       "95",
		IsSystem:    true,
	},
	{
		Name:        "health.node.stale_timeout_seconds",
		Description: "Seconds without heartbeat before a node is considered stale",
		Type:        models.ConfigTypeNumeric,
		Value:       "180",
		IsSystem:    true,
	},
	{
		Name:        "health.metrics.retention_days",
		Description: "Number of days to retain system metrics before automatic deletion",
		Type:        models.ConfigTypeNumeric,
		Value:       "30",
		IsSystem:    true,
	},
	{
		Name:        "log.min_level",
		Description: "Minimum severity level for system log entries. Messages below this level are discarded.",
		Type:        models.ConfigTypeEnum,
		Value:       "debug",
		Options:     `[{"label":"No Logging","value":"none"},{"label":"Critical","value":"critical"},{"label":"High","value":"high"},{"label":"Medium","value":"medium"},{"label":"Low","value":"low"},{"label":"Debug","value":"debug"}]`,
		IsSystem:    true,
	},
}

// Seed inserts any missing system-defined variables into the database.
// Existing variables are not overwritten.
func Seed(ctx context.Context, database *db.MongoDB) error {
	col := database.ConfigVars()
	now := time.Now()

	for _, def := range SystemDefaults {
		err := col.FindOne(ctx, bson.M{"name": def.Name}).Err()
		if err == mongo.ErrNoDocuments {
			def.CreatedAt = now
			def.UpdatedAt = now
			if _, insertErr := col.InsertOne(ctx, def); insertErr != nil {
				return insertErr
			}
			log.Printf("Seeded system config variable: %s", def.Name)
		} else if err != nil {
			return err
		}
	}
	return nil
}
