package handlers

import legacy "backend/handlers"

var (
	GetUsers    = legacy.GetUsers
	GetUserByID = legacy.GetUserByID
	CreateUser  = legacy.CreateUser
	UpdateUser  = legacy.UpdateUser
	DeleteUser  = legacy.DeleteUser
)
