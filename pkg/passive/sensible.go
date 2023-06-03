package passive

import (
	cregex "github.com/mingrammer/commonregex"
)

type SensibleData struct {
	Type  string
	Value string
}

// GetSensibleDataFromText gets sensible data from a string and returns it as a SensibleData slice
func GetSensibleDataFromText(text string) (findings []SensibleData) {
	emails := cregex.Emails(text)
	for _, email := range emails {
		findings = append(findings, SensibleData{
			Type:  "email",
			Value: email,
		})
	}
	phoneNumbers := cregex.Phones(text)
	for _, phoneNumber := range phoneNumbers {
		findings = append(findings, SensibleData{
			Type:  "phone-number",
			Value: phoneNumber,
		})
	}
	phoneNumbersWithExt := cregex.PhonesWithExts(text)
	for _, phoneNumber := range phoneNumbersWithExt {
		findings = append(findings, SensibleData{
			Type:  "phone-number",
			Value: phoneNumber,
		})
	}
	ips := cregex.IPs(text)
	for _, ip := range ips {
		findings = append(findings, SensibleData{
			Type:  "ip",
			Value: ip,
		})
	}
	macs := cregex.MACAddresses(text)
	for _, mac := range macs {
		findings = append(findings, SensibleData{
			Type:  "mac-address",
			Value: mac,
		})
	}
	creditCardNumbers := cregex.CreditCards(text)
	for _, card := range creditCardNumbers {
		findings = append(findings, SensibleData{
			Type:  "credit-card-number",
			Value: card,
		})
	}
	gitRepos := cregex.GitRepos(text)
	for _, repo := range gitRepos {
		findings = append(findings, SensibleData{
			Type:  "git-repository",
			Value: repo,
		})
	}
	btcAddresses := cregex.BtcAddresses(text)
	for _, btc := range btcAddresses {
		findings = append(findings, SensibleData{
			Type:  "btc-address",
			Value: btc,
		})
	}
	guids := cregex.GUIDs(text)
	for _, guid := range guids {
		findings = append(findings, SensibleData{
			Type:  "guid",
			Value: guid,
		})
	}
	for _, finding := range GetHashesFromText(text) {
		findings = append(findings, finding)
	}

	return findings
}

func GetHashesFromText(text string) (findings []SensibleData) {
	sha1Hashes := cregex.SHA1Hexes(text)
	for _, sha1 := range sha1Hashes {
		findings = append(findings, SensibleData{
			Type:  "sha1-hash",
			Value: sha1,
		})
	}
	sha256Hashes := cregex.SHA256Hexes(text)
	for _, sha256 := range sha256Hashes {
		findings = append(findings, SensibleData{
			Type:  "sha256-hash",
			Value: sha256,
		})
	}
	md5Hashes := cregex.MD5Hexes(text)
	for _, md5 := range md5Hashes {
		findings = append(findings, SensibleData{
			Type:  "md5-hash",
			Value: md5,
		})
	}
	return findings
}
