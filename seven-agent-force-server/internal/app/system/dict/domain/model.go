package domain

import "time"

type DictType struct {
	ID             int64      `json:"id"`
	DictCode       string     `json:"dictCode"`
	DictName       string     `json:"dictName"`
	DictDesc       string     `json:"dictDesc,omitempty"`
	Module         string     `json:"module,omitempty"`
	Status         int        `json:"status"`
	RequiredLogin  int        `json:"requiredLogin"`
	ValueType      string     `json:"valueType"`
	UIWidget       string     `json:"uiWidget"`
	ValidationJSON string     `json:"validationJson,omitempty"`
	Exposure       string     `json:"exposure"`
	Sensitivity    string     `json:"sensitivity"`
	SchemaVersion  int        `json:"schemaVersion"`
	Version        int64      `json:"version"`
	IsSystem       int        `json:"isSystem"`
	SortOrder      int        `json:"sortOrder"`
	CreatedBy      int64      `json:"createdBy,omitempty"`
	CreateTime     *time.Time `json:"createTime,omitempty"`
	UpdatedBy      int64      `json:"updatedBy,omitempty"`
	UpdateTime     *time.Time `json:"updateTime,omitempty"`
	IsDeleted      int        `json:"isDeleted"`
	ItemCount      int64      `json:"itemCount"`
}

type DictItem struct {
	ID                  int64      `json:"id"`
	DictTypeID          int64      `json:"dictTypeId"`
	ItemValue           string     `json:"itemValue"`
	ItemLabel           string     `json:"itemLabel"`
	ItemDesc            string     `json:"itemDesc,omitempty"`
	SortOrder           int        `json:"sortOrder"`
	Status              int        `json:"status"`
	ExtJSON             string     `json:"extJson,omitempty"`
	ColorToken          string     `json:"colorToken,omitempty"`
	IconToken           string     `json:"iconToken,omitempty"`
	PresentationVersion int        `json:"presentationVersion"`
	Version             int64      `json:"version"`
	CreatedBy           int64      `json:"createdBy,omitempty"`
	CreateTime          *time.Time `json:"createTime,omitempty"`
	UpdatedBy           int64      `json:"updatedBy,omitempty"`
	UpdateTime          *time.Time `json:"updateTime,omitempty"`
	IsDeleted           int        `json:"isDeleted"`
}

type DictItemView struct {
	DictItem
	DictCode string `json:"dictCode,omitempty"`
	DictName string `json:"dictName,omitempty"`
}

type BatchResult struct {
	Record  map[string][]DictItemView `json:"record"`
	Missing []string                  `json:"missing,omitempty"`
}

type DictTypePageQuery struct {
	Current  int64
	PageSize int64
	Keyword  string
	Module   string
	Status   *int
}

type DictTypePage struct {
	Current int64
	Size    int64
	Total   int64
	Records []DictType
}

type DictItemListQuery struct {
	DictTypeID int64
	Status     *int
	Keyword    string
}

type CreateDictTypeInput struct {
	DictCode       string
	DictName       string
	DictDesc       string
	Module         string
	Status         *int
	IsSystem       *int
	RequiredLogin  *int
	ValueType      string
	UIWidget       string
	ValidationJSON string
	Exposure       string
	Sensitivity    string
	SchemaVersion  *int
}

type UpdateDictTypeInput struct {
	DictName       *string
	DictDesc       *string
	Module         *string
	Status         *int
	SortOrder      *int
	RequiredLogin  *int
	ValueType      *string
	UIWidget       *string
	ValidationJSON *string
	Exposure       *string
	Sensitivity    *string
	SchemaVersion  *int
}

type CreateDictItemInput struct {
	ItemValue  string
	ItemLabel  string
	ItemDesc   string
	SortOrder  *int
	Status     *int
	ExtJSON    string
	ColorToken string
	IconToken  string
}

type UpdateDictItemInput struct {
	ItemLabel  *string
	ItemDesc   *string
	SortOrder  *int
	Status     *int
	ExtJSON    *string
	ColorToken *string
	IconToken  *string
}
