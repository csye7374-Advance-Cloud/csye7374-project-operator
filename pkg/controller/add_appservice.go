package controller

import (
	"github.com/csye7374-Advance-Cloud/csye7374-project-operator/pkg/controller/appservice"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, appservice.Add)
}
