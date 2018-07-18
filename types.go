package main

type User struct {
	Id         string `json:"id,omitempty"`
	Username   string `json:"username,omitempty"`
	AvatarHash string `json:"avatarHash,omitempty"`
	Avatar     string `json:"avatar,omitempty"`
	FullName   string `json:"fullName,omitempty"`
}

type Board struct {
	Id        string `db:"id" json:"id,omitempty"`
	ShortLink string `json:"shortLink,omitempty"`
	Name      string `json:"name,omitempty"`

	Token   string `db:"token" json:"-,omitempty"`
	Enabled bool   `db:"enabled" json:"enabled,omitempty"`
}

type List struct {
	Id   string  `json:"id,omitempty"`
	Name string  `json:"name,omitempty"`
	Pos  float64 `json:"pos,omitempty"`
}

type Label struct {
	Id      string `json:"id,omitempty"`
	Name    string `json:"name"`
	Color   string `json:"color,omitempty"`
	IdBoard string `json:"idBoard,omitempty"`
}

type Card struct {
	Id                string            `json:"id,omitempty"`
	ShortLink         string            `json:"shortLink,omitempty"`
	IdBoard           string            `json:"idBoard,omitempty"`
	IdList            string            `json:"idList,omitempty"`
	Name              string            `json:"name,omitempty"`
	Desc              string            `json:"desc,omitempty"`
	Due               string            `json:"due,omitempty"`
	DueComplete       bool              `json:"dueComplete,omitempty"`
	Closed            bool              `json:"closed,omitempty"`
	Pos               float64           `json:"pos,omitempty"`
	IdAttachmentCover string            `json:"idAttachmentCover,omitempty"`
	IdMembers         []string          `json:"idMembers,omitempty"`
	IdLabels          []string          `json:"idLabels,omitempty"`
	IdChecklists      []string          `json:"idChecklists,omitempty"`
	IdAttachments     []string          `json:"idAttachments,omitempty"`
	Checklists        []Checklist       `json:"checklists,omitempty"`
	Attachments       []Attachment      `json:"attachments,omitempty"`
	CustomFieldItems  []CustomFieldItem `json:"customFieldItems,omitempty"`
}

type Checklist struct {
	Id           string      `json:"id,omitempty"`
	Name         string      `json:"name,omitempty"`
	CheckItems   []CheckItem `json:"checkItems,omitempty"`
	IdCheckItems []string    `json:"idCheckItems,omitempty"`
}

type CheckItem struct {
	Id      string  `json:"id,omitempty"`
	Name    string  `json:"name,omitempty"`
	State   string  `json:"state,omitempty"`
	Checked bool    `json:"checked,omitempty"`
	Pos     float64 `json:"pos,omitempty"`
}

type Attachment struct {
	Id   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	Url  string `json:"url,omitempty"`
}

type Comment struct {
	Id   string `json:"id,omitempty"`
	Text string `json:"text,omitempty"`
}

type IdName struct {
	Id   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type Webhook struct {
	Action Action `json:"action,omitempty"`
	Model  Model  `json:"model,omitempty"`
}

type Action struct {
	Type          string `json:"type,omitempty"`
	Date          string `json:"date,omitempty"`
	Id            string `json:"id,omitempty"`
	Data          Data   `json:data`
	MemberCreator User   `json:"memberCreator,omitempty"`
}

type Data struct {
	ListBefore      IdName                 `json:"listBefore,omitempty"`
	ListAfter       IdName                 `json:"listAfter,omitempty"`
	List            List                   `json:"list,omitempty"`
	Label           Label                  `json:"label,omitempty"`
	Board           Board                  `json:"board,omitempty"`
	BoardSource     Board                  `json:"boardSource,omitempty"`
	BoardTarget     Board                  `json:"boardTarget,omitempty"`
	Card            Card                   `json:"card,omitempty"`
	Action          Comment                `json:"action,omitempty"`
	Old             map[string]interface{} `json:"old,omitempty"`
	Text            string                 `json:"text,omitempty"`
	Attachment      Attachment             `json:"attachment,omitempty"`
	Checklist       Checklist              `json:"checklist,omitempty"`
	CheckItem       CheckItem              `json:"checkItem,omitempty"`
	CustomFieldItem CustomFieldItem        `json:"customFieldItem,omitempty"`
	CustomField     CustomField            `json:"customField,omitempty"`
	IdMember        string                 `json:"idMember,omitempty"`
}

type CustomField struct {
	Id      string            `json:"id,omitempty"`
	Type    string            `json:"type,omitempty"` // text, date, number, checkbox, list
	Name    string            `json:"name,omitempty"`
	Options []CustomFieldItem `json:"options,omitempty"`
}

type CustomFieldValue struct {
	Text    string `json:"text,omitempty,omitempty"`
	Date    string `json:"date,omitempty,omitempty"`
	Number  string `json:"number,omitempty,omitempty"`
	Checked string `json:"checked,omitempty,omitempty"`
}

type CustomFieldItem struct {
	Id string `json:"id,omitempty,omitempty"`

	// opt value for 'list' fields
	IdValue string `json:"idValue,omitempty,omitempty"`

	// other types will have this
	Value *CustomFieldValue `json:"value,omitempty,omitempty"`

	Color         string `json:"color,omitempty"`
	IdCustomField string `json:"idCustomField,omitempty,omitempty"`
}

type Model struct {
	Id string `json:"id,omitempty"`
}

type Value struct {
	Value string `json:"value"`
}

type Result struct {
	Ok    bool   `json:"ok,omitempty"`
	Value string `json:"value,omitempty"`
	Error string `json:"error,omitempty"`
}
