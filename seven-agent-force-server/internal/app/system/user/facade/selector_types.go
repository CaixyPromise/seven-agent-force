package facade

// UserSelectorQuery describes a bounded user lookup for selector controls.
type UserSelectorQuery struct {
	Keyword string
	Limit   int
	DeptID  int64
	Scope   DataScopeFilter
}

// SimpleUserVO is the minimum user projection exposed to selector controls.
type SimpleUserVO struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	NickName string `json:"nickName"`
	Avatar   string `json:"avatar,omitempty"`
	Status   int    `json:"status"`
}
