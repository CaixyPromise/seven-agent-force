package facade

type SetupStatusDTO struct {
	Initialized   bool    `json:"initialized"`
	OwnerRequired bool    `json:"ownerRequired"`
	LoginEnabled  bool    `json:"loginEnabled"`
	AppVersion    string  `json:"appVersion"`
	AppCommit     string  `json:"appCommit"`
	StartTime     string  `json:"startTime"`
	SetupToken    *string `json:"setupToken"`
}

type SetupOwnerRequestDTO struct {
	Username        string `json:"username"`
	Nickname        string `json:"nickname"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirmPassword"`
}

type SetupOwnerResultDTO struct {
	ID           int64    `json:"id"`
	Username     string   `json:"username"`
	Nickname     string   `json:"nickname"`
	UserAvatar   string   `json:"userAvatar"`
	Permissions  []string `json:"permissions"`
	RoleCodes    []string `json:"roleCodes"`
	AccessToken  string   `json:"accessToken"`
	TokenType    string   `json:"tokenType"`
	AccessTTLSec int64    `json:"accessTtlSec"`
}

type OwnerBootstrapResult struct {
	Owner                    *SetupOwnerResultDTO
	SessionCookieHeaderValue string
	RefreshCookieHeaderValue string
}
