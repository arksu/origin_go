package restapi

type RegistrationRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type CreateCharacterRequest struct {
	Name string `json:"name"`
}
