package onvif

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// AnalyticsRule represents an analytics rule on the device.
type AnalyticsRule struct {
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Parameters map[string]string `json:"parameters"`
}

// AnalyticsModule represents an analytics module on the device.
type AnalyticsModule struct {
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Parameters map[string]string `json:"parameters"`
}

// --- SOAP response types for analytics ---

type analyticsEnvelope struct {
	XMLName xml.Name      `xml:"Envelope"`
	Body    analyticsBody `xml:"Body"`
}

type analyticsBody struct {
	GetSupportedRulesResponse    *getSupportedRulesResponse    `xml:"GetSupportedRulesResponse"`
	GetRulesResponse             *getRulesResponse             `xml:"GetRulesResponse"`
	GetAnalyticsModulesResponse  *getAnalyticsModulesResponse  `xml:"GetAnalyticsModulesResponse"`
	Fault                        *analyticsFault               `xml:"Fault"`
}

type analyticsFault struct {
	Faultstring string `xml:"faultstring"`
}

type getSupportedRulesResponse struct {
	SupportedRules supportedRules `xml:"SupportedRules"`
}

type supportedRules struct {
	RuleDescription []ruleDescription `xml:"RuleContentSchemaLocation"`
	RuleTypes       []ruleType        `xml:"RuleDescription"`
}

type ruleDescription struct {
	Value string `xml:",chardata"`
}

type ruleType struct {
	RuleName string `xml:"Name,attr"`
	RuleType string `xml:"Type,attr"`
}

type getRulesResponse struct {
	Rule []analyticsRuleXML `xml:"Rule"`
}

type analyticsRuleXML struct {
	Name       string               `xml:"Name,attr"`
	Type       string               `xml:"Type,attr"`
	Parameters analyticsParameters  `xml:"Parameters"`
}

type analyticsParameters struct {
	SimpleItems  []analyticsSimpleItem  `xml:"SimpleItem"`
	ElementItems []analyticsElementItem `xml:"ElementItem"`
}

type analyticsSimpleItem struct {
	Name  string `xml:"Name,attr"`
	Value string `xml:"Value,attr"`
}

type analyticsElementItem struct {
	Name    string `xml:"Name,attr"`
	Content string `xml:",innerxml"`
}

type getAnalyticsModulesResponse struct {
	AnalyticsModule []analyticsRuleXML `xml:"AnalyticsModule"`
}

// --- SOAP helper ---

// analyticsSoap builds a SOAP envelope with the tan namespace for Analytics requests.
func analyticsSoap(innerBody string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:tan="http://www.onvif.org/ver20/analytics/wsdl"
            xmlns:onvif="http://www.onvif.org/ver10/schema">
  <s:Header></s:Header>
  <s:Body>
    %s
  </s:Body>
</s:Envelope>`, innerBody)
}

// doAnalyticsSOAP sends a SOAP request to the analytics service endpoint.
func doAnalyticsSOAP(xaddr, username, password, body string) ([]byte, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, fmt.Errorf("connect to ONVIF device: %w", err)
	}

	analyticsURL := client.ServiceURL("analytics")
	if analyticsURL == "" {
		return nil, fmt.Errorf("device does not support analytics service")
	}

	soapBody := analyticsSoap(body)

	// Inject WS-Security if credentials are present.
	if username != "" {
		soapBody = injectWSSecurity(soapBody, username, password)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, analyticsURL, strings.NewReader(soapBody))
	if err != nil {
		return nil, fmt.Errorf("create analytics request: %w", err)
	}
	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("analytics http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("analytics read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("analytics SOAP fault (HTTP %d): %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	return respBody, nil
}

// buildSimpleItems generates the XML for simple item parameters.
func buildSimpleItems(params map[string]string) string {
	var sb strings.Builder
	for k, v := range params {
		sb.WriteString(fmt.Sprintf(`<onvif:SimpleItem Name="%s" Value="%s"/>`, xmlEscape(k), xmlEscape(v)))
	}
	return sb.String()
}

// xmlEscape escapes XML special characters in attribute values.
func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// --- Public functions ---

// GetSupportedRules returns the list of supported analytics rule type names.
func GetSupportedRules(xaddr, username, password, configToken string) ([]string, error) {
	reqBody := fmt.Sprintf(`<tan:GetSupportedRules>
      <tan:ConfigurationToken>%s</tan:ConfigurationToken>
    </tan:GetSupportedRules>`, configToken)

	body, err := doAnalyticsSOAP(xaddr, username, password, reqBody)
	if err != nil {
		return nil, fmt.Errorf("GetSupportedRules: %w", err)
	}

	var env analyticsEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("GetSupportedRules parse: %w", err)
	}
	if env.Body.Fault != nil {
		return nil, fmt.Errorf("GetSupportedRules SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.GetSupportedRulesResponse == nil {
		return nil, fmt.Errorf("GetSupportedRules: empty response")
	}

	var names []string
	for _, rt := range env.Body.GetSupportedRulesResponse.SupportedRules.RuleTypes {
		name := rt.RuleName
		if name == "" {
			name = rt.RuleType
		}
		if name != "" {
			names = append(names, name)
		}
	}

	return names, nil
}

// GetRules returns all analytics rules for the given configuration token.
func GetRules(xaddr, username, password, configToken string) ([]AnalyticsRule, error) {
	reqBody := fmt.Sprintf(`<tan:GetRules>
      <tan:ConfigurationToken>%s</tan:ConfigurationToken>
    </tan:GetRules>`, configToken)

	body, err := doAnalyticsSOAP(xaddr, username, password, reqBody)
	if err != nil {
		return nil, fmt.Errorf("GetRules: %w", err)
	}

	var env analyticsEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("GetRules parse: %w", err)
	}
	if env.Body.Fault != nil {
		return nil, fmt.Errorf("GetRules SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.GetRulesResponse == nil {
		return nil, fmt.Errorf("GetRules: empty response")
	}

	return convertRules(env.Body.GetRulesResponse.Rule), nil
}

// CreateRule creates an analytics rule on the device.
func CreateRule(xaddr, username, password, configToken string, rule AnalyticsRule) error {
	simpleItems := buildSimpleItems(rule.Parameters)
	reqBody := fmt.Sprintf(`<tan:CreateRules>
      <tan:ConfigurationToken>%s</tan:ConfigurationToken>
      <tan:Rule Name="%s" Type="%s">
        <onvif:Parameters>%s</onvif:Parameters>
      </tan:Rule>
    </tan:CreateRules>`, configToken, xmlEscape(rule.Name), xmlEscape(rule.Type), simpleItems)

	_, err := doAnalyticsSOAP(xaddr, username, password, reqBody)
	if err != nil {
		return fmt.Errorf("CreateRule: %w", err)
	}
	return nil
}

// ModifyRule modifies an existing analytics rule on the device.
func ModifyRule(xaddr, username, password, configToken string, rule AnalyticsRule) error {
	simpleItems := buildSimpleItems(rule.Parameters)
	reqBody := fmt.Sprintf(`<tan:ModifyRules>
      <tan:ConfigurationToken>%s</tan:ConfigurationToken>
      <tan:Rule Name="%s" Type="%s">
        <onvif:Parameters>%s</onvif:Parameters>
      </tan:Rule>
    </tan:ModifyRules>`, configToken, xmlEscape(rule.Name), xmlEscape(rule.Type), simpleItems)

	_, err := doAnalyticsSOAP(xaddr, username, password, reqBody)
	if err != nil {
		return fmt.Errorf("ModifyRule: %w", err)
	}
	return nil
}

// DeleteRule deletes an analytics rule from the device.
func DeleteRule(xaddr, username, password, configToken, ruleName string) error {
	reqBody := fmt.Sprintf(`<tan:DeleteRules>
      <tan:ConfigurationToken>%s</tan:ConfigurationToken>
      <tan:RuleName>%s</tan:RuleName>
    </tan:DeleteRules>`, configToken, xmlEscape(ruleName))

	_, err := doAnalyticsSOAP(xaddr, username, password, reqBody)
	if err != nil {
		return fmt.Errorf("DeleteRule: %w", err)
	}
	return nil
}

// GetAnalyticsModules returns all analytics modules for the given configuration token.
func GetAnalyticsModules(xaddr, username, password, configToken string) ([]AnalyticsModule, error) {
	reqBody := fmt.Sprintf(`<tan:GetAnalyticsModules>
      <tan:ConfigurationToken>%s</tan:ConfigurationToken>
    </tan:GetAnalyticsModules>`, configToken)

	body, err := doAnalyticsSOAP(xaddr, username, password, reqBody)
	if err != nil {
		return nil, fmt.Errorf("GetAnalyticsModules: %w", err)
	}

	var env analyticsEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("GetAnalyticsModules parse: %w", err)
	}
	if env.Body.Fault != nil {
		return nil, fmt.Errorf("GetAnalyticsModules SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.GetAnalyticsModulesResponse == nil {
		return nil, fmt.Errorf("GetAnalyticsModules: empty response")
	}

	var modules []AnalyticsModule
	for _, m := range env.Body.GetAnalyticsModulesResponse.AnalyticsModule {
		mod := AnalyticsModule{
			Name:       m.Name,
			Type:       m.Type,
			Parameters: make(map[string]string),
		}
		for _, item := range m.Parameters.SimpleItems {
			mod.Parameters[item.Name] = item.Value
		}
		modules = append(modules, mod)
	}

	return modules, nil
}

// convertRules converts XML rule representations to the public AnalyticsRule type.
func convertRules(xmlRules []analyticsRuleXML) []AnalyticsRule {
	var rules []AnalyticsRule
	for _, r := range xmlRules {
		rule := AnalyticsRule{
			Name:       r.Name,
			Type:       r.Type,
			Parameters: make(map[string]string),
		}
		for _, item := range r.Parameters.SimpleItems {
			rule.Parameters[item.Name] = item.Value
		}
		rules = append(rules, rule)
	}
	return rules
}
