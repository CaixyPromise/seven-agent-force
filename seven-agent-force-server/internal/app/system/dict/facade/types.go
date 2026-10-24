package facade

import "time"

type PageResult[T any] struct {
	Current int64 `json:"current"`
	Size    int64 `json:"size"`
	Total   int64 `json:"total"`
	Records []T   `json:"records"`
}

type DictTypeAddRequest struct {
	DictCode      string         `json:"dictCode" validate:"required,min=2,max=50"`
	DictName      string         `json:"dictName" validate:"required"`
	DictDesc      string         `json:"dictDesc,omitempty"`
	Module        string         `json:"module,omitempty"`
	Status        *int           `json:"status,omitempty"`
	IsSystem      *int           `json:"isSystem,omitempty"`
	RequiredLogin *int           `json:"requiredLogin,omitempty"`
	ValueType     string         `json:"valueType,omitempty"`
	UIWidget      string         `json:"uiWidget,omitempty"`
	Validation    map[string]any `json:"validation,omitempty"`
	Exposure      string         `json:"exposure,omitempty"`
	Sensitivity   string         `json:"sensitivity,omitempty"`
	SchemaVersion *int           `json:"schemaVersion,omitempty"`
}

type DictTypeUpdateRequest struct {
	ID            int64          `json:"id" validate:"required"`
	DictName      *string        `json:"dictName,omitempty"`
	DictDesc      *string        `json:"dictDesc,omitempty"`
	Module        *string        `json:"module,omitempty"`
	Status        *int           `json:"status,omitempty"`
	SortOrder     *int           `json:"sortOrder,omitempty"`
	RequiredLogin *int           `json:"requiredLogin,omitempty"`
	ValueType     *string        `json:"valueType,omitempty"`
	UIWidget      *string        `json:"uiWidget,omitempty"`
	Validation    map[string]any `json:"validation,omitempty"`
	Exposure      *string        `json:"exposure,omitempty"`
	Sensitivity   *string        `json:"sensitivity,omitempty"`
	SchemaVersion *int           `json:"schemaVersion,omitempty"`
	Version       *int64         `json:"version,omitempty"`
}

type DictTypeQueryRequest struct {
	Current   int64  `json:"current,omitempty" query:"current"`
	PageNum   int64  `json:"pageNum,omitempty" query:"pageNum"`
	PageSize  int64  `json:"pageSize,omitempty" query:"pageSize"`
	SortField string `json:"sortField,omitempty" query:"sortField"`
	SortOrder string `json:"sortOrder,omitempty" query:"sortOrder"`
	Keyword   string `json:"keyword,omitempty" query:"keyword"`
	Module    string `json:"module,omitempty" query:"module"`
	Status    *int   `json:"status,omitempty" query:"status"`
}

type DictTypeVO struct {
	ID            int64          `json:"id"`
	DictCode      string         `json:"dictCode"`
	DictName      string         `json:"dictName"`
	DictDesc      string         `json:"dictDesc,omitempty"`
	Module        string         `json:"module,omitempty"`
	Status        int            `json:"status"`
	RequiredLogin int            `json:"requiredLogin"`
	ValueType     string         `json:"valueType"`
	UIWidget      string         `json:"uiWidget"`
	Validation    map[string]any `json:"validation,omitempty"`
	Exposure      string         `json:"exposure"`
	Sensitivity   string         `json:"sensitivity"`
	SchemaVersion int            `json:"schemaVersion"`
	Version       int64          `json:"version"`
	IsSystem      int            `json:"isSystem"`
	CreatedBy     int64          `json:"createdBy,omitempty"`
	CreateTime    *time.Time     `json:"createTime,omitempty"`
	UpdatedBy     int64          `json:"updatedBy,omitempty"`
	UpdateTime    *time.Time     `json:"updateTime,omitempty"`
	ItemCount     int64          `json:"itemCount"`
	SortOrder     int            `json:"sortOrder"`
}

type DictItemAddRequest struct {
	ItemValue  string `json:"itemValue" validate:"required"`
	ItemLabel  string `json:"itemLabel" validate:"required"`
	ItemDesc   string `json:"itemDesc,omitempty"`
	SortOrder  *int   `json:"sortOrder,omitempty"`
	Status     *int   `json:"status,omitempty"`
	ExtJSON    string `json:"extJson,omitempty"`
	ColorToken string `json:"colorToken,omitempty"`
	IconToken  string `json:"iconToken,omitempty"`
}

type DictItemUpdateRequest struct {
	ID         int64   `json:"id" validate:"required"`
	ItemLabel  *string `json:"itemLabel,omitempty"`
	ItemDesc   *string `json:"itemDesc,omitempty"`
	SortOrder  *int    `json:"sortOrder,omitempty"`
	Status     *int    `json:"status,omitempty"`
	ExtJSON    *string `json:"extJson,omitempty"`
	ColorToken *string `json:"colorToken,omitempty"`
	IconToken  *string `json:"iconToken,omitempty"`
	Version    *int64  `json:"version,omitempty"`
}

type DictItemQueryRequest struct {
	DictTypeID int64  `json:"dictTypeId,omitempty" query:"dictTypeId"`
	Force      bool   `json:"force,omitempty" query:"force"`
	Status     *int   `json:"status,omitempty" query:"status"`
	Keyword    string `json:"keyword,omitempty" query:"keyword"`
}

type DictItemSortRequest struct {
	Items []DictItemSortItem `json:"items" validate:"required,min=1,dive"`
}

type DictItemSortItem struct {
	ID        int64 `json:"id" validate:"required"`
	SortOrder int   `json:"sortOrder" validate:"required"`
}

type DictItemVO struct {
	ID                  int64      `json:"id"`
	DictTypeID          int64      `json:"dictTypeId"`
	DictCode            string     `json:"dictCode,omitempty"`
	DictName            string     `json:"dictName,omitempty"`
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
}

type MoveRequest struct {
	BeforeID *int64 `json:"beforeId,omitempty"`
	AfterID  *int64 `json:"afterId,omitempty"`
}

type DictBatchRequest struct {
	DictCodes []string `json:"dictCodes" validate:"required,min=1,max=30,dive,required"`
	Force     bool     `json:"force,omitempty"`
}

type DictBatchResponse struct {
	Record  map[string][]DictItemVO `json:"record"`
	Missing []string                `json:"missing,omitempty"`
}

type DictInternalReadRequest struct {
	ConsumerID         string `json:"consumerId"`
	DictCode           string `json:"dictCode"`
	ServerScope        string `json:"serverScope"`
	Purpose            string `json:"purpose"`
	AllowedSensitivity string `json:"allowedSensitivity"`
}

type DictInternalBatchReadRequest struct {
	ConsumerID         string   `json:"consumerId"`
	DictCodes          []string `json:"dictCodes"`
	ServerScope        string   `json:"serverScope"`
	Purpose            string   `json:"purpose"`
	AllowedSensitivity string   `json:"allowedSensitivity"`
}

type DictInternalListRequest struct {
	ConsumerID         string `json:"consumerId"`
	ServerScope        string `json:"serverScope"`
	Purpose            string `json:"purpose"`
	AllowedSensitivity string `json:"allowedSensitivity"`
}

type DictConsumerRegistration struct {
	ConsumerID         string `json:"consumerId"`
	DictCode           string `json:"dictCode"`
	ServerScope        string `json:"serverScope"`
	Purpose            string `json:"purpose"`
	AllowedSensitivity string `json:"allowedSensitivity"`
	Source             string `json:"source"`
	ActualConsumer     string `json:"actualConsumer"`
	Activation         string `json:"activation"`
	CacheRule          string `json:"cacheRule"`
}

type DictConsumerVO struct {
	DictConsumerRegistration
	Connected bool `json:"connected"`
}
