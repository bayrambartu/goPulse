package user

type User struct {
	Name    string `json:"name" binding:"required,min=2"`
	Surname string `json:"surname" binding:"required,min=2"`
}
