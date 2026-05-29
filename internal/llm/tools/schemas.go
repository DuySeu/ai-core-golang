package tools

// RegisterTools creates all built-in tool definitions.
func RegisterTools(extra ...*Tool) []*Tool {
	toolList := []*Tool{
		NewTool("get_stock_price",
			"Get OHLC stock price data for a Vietnamese stock symbol from VietCap.",
			HandleGetStockPrice,
		),
		NewTool("get_report",
			"Get quarterly or yearly financial report for a stock.",
			HandleGetReport,
		),
	}
	return append(toolList, extra...)
}
