package importer

import (
	"encoding/json"
	"strings"
)

type MembersResults struct {
	Members    []Member `json:"members"`
	TotalItems int      `json:"total_items"`
}

type Member struct {
	Id           string      `json:"id"`
	EmailAddress string      `json:"email_address"`
	FullName     string      `json:"full_name"`
	Status       string      `json:"status"`
	MergeFields  MergeFields `json:"merge_fields"`
}

type MergeFields struct {
	FirstName string `json:"FNAME"`
	LastName  string `json:"LNAME"`
}

type ometriaMember struct {
	Id        string `json:"id"`
	FirstName string `json:"firstname"`
	LastName  string `json:"lastname"`
	Email     string `json:"email"`
	Status    string `json:"status"`
}

func (member Member) MarshalJSON() ([]byte, error) {

	firstName := member.MergeFields.FirstName
	lastName := member.MergeFields.LastName
	fullName := member.FullName

	// if first name is not present on merge_fields, derive it from full_name
	if len(strings.TrimSpace(firstName)) == 0 && len(strings.TrimSpace(fullName)) > 0 {
		parts := strings.Split(fullName, " ")
		if len(parts) >= 1 {
			firstName = parts[0]
		}
	}

	// if last name is not present on merge_fields, derive it from full_name
	if len(strings.TrimSpace(lastName)) == 0 && len(strings.TrimSpace(fullName)) > 0 {
		parts := strings.Split(fullName, " ")
		if len(parts) > 1 {
			lastName = parts[len(parts)-1]
		}
	}

	return json.Marshal(ometriaMember{
		Id:        member.Id,
		FirstName: firstName,
		LastName:  lastName,
		Email:     member.EmailAddress,
		Status:    member.Status,
	})
}
