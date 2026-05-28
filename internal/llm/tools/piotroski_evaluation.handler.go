package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// GraphQLPayload represents the GraphQL request payload
type GraphQLPayload struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

type PiotroskiEvaluation struct {
	Symbol  string  `json:"symbol"`
	Period  string  `json:"period"`
	Score   int     `json:"score"`
	Details Details `json:"details"`
}

type Details struct {
	NetIncome              bool `json:"net_income"`
	ROA                    bool `json:"roa"`
	NetOperatingCashFlow   bool `json:"net_operating_cash_flow"`
	CashFlowFromOperations bool `json:"cash_flow_from_operations"`
	LongTermDebt           bool `json:"long_term_debt"`
	CurrentRatio           bool `json:"current_ratio"`
	NewsIssued             bool `json:"news_issued"`
	GrossMargin            bool `json:"gross_margin"`
	AssetTurnoverRatio     bool `json:"asset_turnover_ratio"`
}

const piotroskiQuery = "fragment Ratios on CompanyFinancialRatio { ticker yearReport revenue revenueGrowth netProfit roa currentRatio grossMargin at issueShare pe eps pcf de le __typename } query Query($ticker: String!, $period: String!) { CompanyFinancialRatio(ticker: $ticker, period: $period) { ratio { ...Ratios __typename } period __typename } }"

type PiotroskiInput struct {
	Symbol string `json:"symbol" jsonschema:"Stock symbol, e.g., HPG"`
}

func GetPiotroskiEvaluation(ctx context.Context, input PiotroskiInput) (any, PiotroskiEvaluation, error) {
	if input.Symbol == "" {
		return nil, PiotroskiEvaluation{}, fmt.Errorf("symbol is required")
	}

	payload := GraphQLPayload{
		Query: piotroskiQuery,
		Variables: map[string]interface{}{
			"ticker": input.Symbol,
			"period": "Q",
		},
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, PiotroskiEvaluation{}, fmt.Errorf("failed to marshal payload: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", GRAPHQL_URL, bytes.NewBuffer(payloadJSON))
	if err != nil {
		return nil, PiotroskiEvaluation{}, err
	}
	for k, v := range VCI_HEADERS {
		httpReq.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, PiotroskiEvaluation{}, err
	}
	defer resp.Body.Close()

	reader, err := GZIPCompression(resp.Body, resp.Header.Get("Content-Encoding"))
	if err != nil {
		return nil, PiotroskiEvaluation{}, err
	}
	defer reader.Close()

	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, PiotroskiEvaluation{}, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, PiotroskiEvaluation{}, err
	}

	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return nil, PiotroskiEvaluation{}, fmt.Errorf("invalid response format: data field missing")
	}
	financialRatio, ok := data["CompanyFinancialRatio"].(map[string]interface{})
	if !ok {
		return nil, PiotroskiEvaluation{}, fmt.Errorf("invalid response format: CompanyFinancialRatio missing")
	}
	ratios, ok := financialRatio["ratio"].([]interface{})
	if !ok {
		return nil, PiotroskiEvaluation{}, fmt.Errorf("invalid response format: ratio missing")
	}
	periods, ok := financialRatio["period"].([]interface{})
	if !ok {
		return nil, PiotroskiEvaluation{}, fmt.Errorf("invalid response format: period missing")
	}

	if len(periods) == 0 || len(ratios) == 0 {
		return nil, PiotroskiEvaluation{}, fmt.Errorf("no financial data found")
	}

	latestPeriod := periods[0].(string)

	if len(ratios) < 5 {
		return nil, PiotroskiEvaluation{}, fmt.Errorf("insufficient data for year-over-year comparison (need at least 5 quarters)")
	}

	current := ratios[0].(map[string]interface{})
	prevYear := ratios[4].(map[string]interface{})

	getFloat := func(m map[string]interface{}, key string) float64 {
		if val, ok := m[key]; ok && val != nil {
			return val.(float64)
		}
		return 0.0
	}

	netIncome := getFloat(current, "netProfit")
	scoreNetIncome := netIncome > 0

	roa := getFloat(current, "roa")
	scoreROA := roa > 0

	pe := getFloat(current, "pe")
	eps := getFloat(current, "eps")
	shares := getFloat(current, "issueShare")
	pcf := getFloat(current, "pcf")

	var ocf float64
	var scoreOCF bool
	if pcf != 0 {
		price := pe * eps
		marketCap := price * shares
		ocf = marketCap / pcf
		scoreOCF = ocf > 0
	}

	scoreCFO := ocf > netIncome
	scoreQualityOfEarnings := ocf > netIncome

	deCurr := getFloat(current, "de")
	dePrev := getFloat(prevYear, "de")
	levCurr := 0.0
	if deCurr != -1 {
		levCurr = deCurr / (1 + deCurr)
	}
	levPrev := 0.0
	if dePrev != -1 {
		levPrev = dePrev / (1 + dePrev)
	}
	scoreLTD := levCurr < levPrev

	crCurr := getFloat(current, "currentRatio")
	crPrev := getFloat(prevYear, "currentRatio")
	scoreCR := crCurr > crPrev

	sharesPrev := getFloat(prevYear, "issueShare")
	scoreDilution := shares <= sharesPrev

	gmCurr := getFloat(current, "grossMargin")
	gmPrev := getFloat(prevYear, "grossMargin")
	scoreGM := gmCurr > gmPrev

	atCurr := getFloat(current, "at")
	atPrev := getFloat(prevYear, "at")
	scoreAT := atCurr > atPrev

	score := 0
	if scoreNetIncome {
		score++
	}
	if scoreROA {
		score++
	}
	if scoreOCF {
		score++
	}
	if scoreQualityOfEarnings {
		score++
	}
	if scoreLTD {
		score++
	}
	if scoreCR {
		score++
	}
	if scoreDilution {
		score++
	}
	if scoreGM {
		score++
	}
	if scoreAT {
		score++
	}

	_ = scoreCFO // included in scoreQualityOfEarnings logic

	evaluation := PiotroskiEvaluation{
		Symbol: input.Symbol,
		Period: latestPeriod,
		Score:  score,
		Details: Details{
			NetIncome:              scoreNetIncome,
			ROA:                    scoreROA,
			NetOperatingCashFlow:   scoreOCF,
			CashFlowFromOperations: scoreCFO,
			LongTermDebt:           scoreLTD,
			CurrentRatio:           scoreCR,
			NewsIssued:             scoreDilution,
			GrossMargin:            scoreGM,
			AssetTurnoverRatio:     scoreAT,
		},
	}
	return nil, evaluation, nil
}
