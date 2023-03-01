package main

import (
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"subscription_service/data"
)

func (app *Config) HomePage(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "home.page.gohtml", nil)
}

func (app *Config) LoginPage(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "login.page.gohtml", nil)
}

func (app *Config) PostLoginPage(w http.ResponseWriter, r *http.Request) {
	_ = app.Session.RenewToken(r.Context())
	// parse from post
	err := r.ParseForm()
	if err != nil {
		app.ErrorLog.Println(err)
	}
	// get email and password
	email := r.Form.Get("email")
	password := r.Form.Get("password")

	user, err := app.Models.User.GetByEmail(email)
	if err != nil {
		app.Session.Put(r.Context(), "error", "Invalid Credentials")
		app.InfoLog.Println("cannot get user by email")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	// check password
	validPassword, err := user.PasswordMatches(password)
	if err != nil {
		app.Session.Put(r.Context(), "error", "Invalid Credentials")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if !validPassword {
		msg := Message{
			To:      email,
			Subject: "Failed log in attempt",
			Data:    "invalid login attempt!",
		}
		app.sendEmail(msg)
		app.Session.Put(r.Context(), "error", "Invalid Credentials")
		app.InfoLog.Println("password does not match")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	// logging user
	app.Session.Put(r.Context(), "userID", user.ID)
	app.Session.Put(r.Context(), "user", user)
	app.Session.Put(r.Context(), "flash", "Successful login")
	// redirect the user
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (app *Config) Logout(w http.ResponseWriter, r *http.Request) {
	// clean up the session
	_ = app.Session.Destroy(r.Context())
	_ = app.Session.RenewToken(r.Context())

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
func (app *Config) RegisterPage(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "register.page.gohtml", nil)
}

func (app *Config) PostRegisterPage(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		app.ErrorLog.Println(err)
	}
	// TODO - validate data
	// create a user
	u := data.User{
		Email:     r.Form.Get("email"),
		FirstName: r.Form.Get("first-name"),
		LastName:  r.Form.Get("last-name"),
		Password:  r.Form.Get("password"),
		Active:    0,
		IsAdmin:   0,
	}
	_, err = u.Insert(u)
	if err != nil {
		app.Session.Put(r.Context(), "error", "unable to create user")
		http.Redirect(w, r, "/register", http.StatusSeeOther)
	}
	// send an activation email
	url := fmt.Sprintf("http://localhost:8090/activate?email=%s", u.Email)
	signedUrl := GenerateTokenFromString(url)
	app.InfoLog.Println(signedUrl)

	msg := Message{
		To:       u.Email,
		Subject:  "Activate your account",
		Template: "confirmation-email",
		Data:     template.HTML(signedUrl),
	}
	app.sendEmail(msg)
	app.Session.Put(r.Context(), "flash", "confirmation email sent. Check your email.")
	http.Redirect(w, r, "/login", http.StatusSeeOther)

	// subscribe the user to an account
}

func (app *Config) ActivateAccount(w http.ResponseWriter, r *http.Request) {
	// validate url
	url := r.RequestURI
	testURL := fmt.Sprintf("http://localhost:8090%s", url)
	okay := VerifyToken(testURL)
	if !okay {
		app.Session.Put(r.Context(), "error", "invalid token")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	// activate account
	u, err := app.Models.User.GetByEmail(r.URL.Query().Get("email"))
	if err != nil {
		app.Session.Put(r.Context(), "error", "no user found")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	u.Active = 1
	err = u.Update()
	if err != nil {
		app.Session.Put(r.Context(), "error", "unable to update the user")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	app.Session.Put(r.Context(), "flash", "account activated. you can now login")
	http.Redirect(w, r, "/login", http.StatusSeeOther)

	// send an email with the invoice attached
}

func (app *Config) SubscribeToPlan(w http.ResponseWriter, r *http.Request) {
	// get the id of the plan that is chosen
	id := r.URL.Query().Get("id")

	planID, _ := strconv.Atoi(id)

	// get the plan from the database
	plan, err := app.Models.Plan.GetOne(planID)
	if err != nil {
		app.Session.Put(r.Context(), "error", "unable to find the plan")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	// get the user from the session
	user, ok := app.Session.Get(r.Context(), "user").(data.User)
	if !ok {
		app.Session.Put(r.Context(), "error", "login first!")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// generate an invoice
	app.Wait.Add(1)

	go func() {
		defer app.Wait.Done()

		invoice, err := app.getInvoice(user, plan)
		if err != nil {
			// send this to a chanel
			app.ErrorChan <- err
		}

		msg := Message{
			To:       user.Email,
			Subject:  "your invoice",
			Data:     invoice,
			Template: "invoice",
		}
		app.sendEmail(msg)
	}()

	// send an email with attachments

	// generate a manual

	// send an email with manual attached

	// subscribe the user to an account

	// redirect
}

func (app *Config) getInvoice(u data.User, plan *data.Plan) (string, error) {
	return plan.PlanAmountFormatted, nil
}
func (app *Config) ChooseSubscription(w http.ResponseWriter, r *http.Request) {
	plans, err := app.Models.Plan.GetAll()
	if err != nil {
		app.ErrorLog.Println(err)
		return
	}
	dataMap := make(map[string]any)
	dataMap["plans"] = plans

	app.render(w, r, "plans.page.gohtml", &TemplateData{
		Data: dataMap,
	})
}
