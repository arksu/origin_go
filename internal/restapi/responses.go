package restapi

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

type LoginResponse struct {
	Token string `json:"token"`
}

type CharacterItem struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type ListCharactersResponse struct {
	List []CharacterItem `json:"list"`
}

type EnterCharacterResponse struct {
	AuthToken string `json:"auth_token"`
}
