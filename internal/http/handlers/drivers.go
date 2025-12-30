package handlers

import legacy "backend/handlers"

var (
	GetDrivers          = legacy.GetDrivers
	CreateDriver        = legacy.CreateDriver
	UpdateDriver        = legacy.UpdateDriver
	DeleteDriver        = legacy.DeleteDriver
	GetDriverAccounts   = legacy.GetDriverAccounts
	CreateDriverAccount = legacy.CreateDriverAccount
	UpdateDriverAccount = legacy.UpdateDriverAccount
	DeleteDriverAccount = legacy.DeleteDriverAccount
)
