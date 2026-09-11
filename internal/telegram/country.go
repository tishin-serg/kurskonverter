package telegram

import (
	"fmt"
	"strings"
)

func parseCountry(value string) (string, error) {
	v := strings.ToUpper(strings.TrimSpace(value))
	switch v {
	case "РОССИЯ", "РОССИЙСКАЯ ФЕДЕРАЦИЯ", "РФ", "RU":
		return "RUS", nil
	case "ГРУЗИЯ", "GE":
		return "GEO", nil
	case "CLEAR":
		return "", nil
	}
	if len(v) != 3 || strings.IndexFunc(v, func(r rune) bool { return r < 'A' || r > 'Z' }) >= 0 {
		return "", fmt.Errorf("введите трёхбуквенный код страны KYC, например RUS или GEO")
	}
	return v, nil
}

func countryLabel(code string) string {
	switch code {
	case "":
		return "не задана"
	case "RUS":
		return "Россия (RUS)"
	case "GEO":
		return "Грузия (GEO)"
	default:
		return code
	}
}
