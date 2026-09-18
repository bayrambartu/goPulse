package user

type CreateUserRequest struct {
	Name    string `json:"name" binding:"required,min=2"`
	Surname string `json:"surname" binding:"required,min=2"`
}

type User struct {
	Name           string
	Surname        string
	Email          string
	HashedPassword string
	Verified       bool
}
