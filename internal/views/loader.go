package views

type LandingPageData struct {
	Message string
}

type LoginPageData struct {
	Message string
	Tracker string
}

type UserDashboardPageData struct {
	CSRFToken string
}

func (x *LandingPageData) toData() map[string]any {
	return map[string]any{
		"Message": x.Message,
	}
}

func (x *LoginPageData) toData() map[string]any {
	return map[string]any{
		"Message": x.Message,
		"Tracker": x.Tracker,
	}
}

func (x *UserDashboardPageData) toData() map[string]any {
	return map[string]any{
		"CSRFToken": x.CSRFToken,
	}
}
