package domain

type HistoryFilter struct {
	Query, Status, Kind, ProductID, AgentID, From, To string
	Page, PageSize                                    int
}
type HistoryPage struct {
	Items    []any `json:"items"`
	Total    int   `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}
