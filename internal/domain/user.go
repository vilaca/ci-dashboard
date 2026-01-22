package domain

// UserProfile represents a user profile from a CI/CD platform.
type UserProfile struct {
	Username  string
	Name      string
	Email     string
	AvatarURL string
	WebURL    string
	Platform  string // CI/CD platform identifier (e.g., "gitlab", "github")
}
