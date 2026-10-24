package domain

// UserSelectorQuery describes a bounded, data-scoped selector lookup.
type UserSelectorQuery struct {
	Keyword string
	Limit   int
	DeptID  int64
	Scope   DataScopeFilter
}

// UserSelectorRecord contains only fields needed to render a user option.
type UserSelectorRecord struct {
	ID          int64  `db:"id"`
	AccountName string `db:"userAccount"`
	NickName    string `db:"nickName"`
	Avatar      string `db:"userAvatar"`
	Status      int    `db:"status"`
}
