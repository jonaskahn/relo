// Status DTOs: the health shapes the management API serves.
package server

import (
	appstatus "github.com/jonaskahn/relo/internal/application/status"
)

type doctorCheckResponse struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

func toDoctorCheckResponse(check appstatus.DoctorCheck) doctorCheckResponse {
	return doctorCheckResponse{
		Name: check.Name, Status: check.Status, Detail: check.Detail,
	}
}

func toDoctorCheckList(checks []appstatus.DoctorCheck) []doctorCheckResponse {
	listed := make([]doctorCheckResponse, 0, len(checks))
	for _, check := range checks {
		listed = append(listed, toDoctorCheckResponse(check))
	}
	return listed
}
