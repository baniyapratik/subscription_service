package main

import (
	"fmt"
	"github.com/phpdave11/gofpdf"
	"github.com/phpdave11/gofpdf/contrib/gofpdi"
	"html/template"
	"net/http"
	"strconv"
	"subscription_service/data"
	"time"
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
}

func (app *Config) SubscribeToPlan(w http.ResponseWriter, r *http.Request) {
	// get the id of the plan that is chosen
	id := r.URL.Query().Get("id")

	planID, err := strconv.Atoi(id)
	if err != nil {
		app.ErrorLog.Println("Error getting plan id: ", err)
	}

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
			app.ErrorLog.Println(err)
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
	app.Wait.Add(1)
	// send an email with attachments
	// generate a manual
	// send an email with manual attached
	go func() {
		defer app.Wait.Done()

		pdf := app.generateManual(user, plan)
		err := pdf.OutputFileAndClose(fmt.Sprintf("./tmp/%d_manual.pdf", user.ID))
		if err != nil {
			app.ErrorLog.Println(err)
			app.ErrorChan <- err
			return
		}

		msg := Message{
			To:      user.Email,
			Subject: "your manual",
			Data:    "your user manual is attached",
			AttachmentMap: map[string]string{
				"Manual.pdf": fmt.Sprintf("./tmp/%d_manual.pdf", user.ID),
			},
		}
		app.sendEmail(msg)

	}()

	// subscribe the user to a plan
	err = app.Models.Plan.SubscribeUserToPlan(user, *plan)
	if err != nil {
		app.Session.Put(r.Context(), "error", "error subscribing to plan")
		http.Redirect(w, r, "/members/plan", http.StatusSeeOther)
		return
	}
	u, err := app.Models.User.GetOne(user.ID)
	if err != nil {
		app.Session.Put(r.Context(), "error", "error subscribing to plan")
		http.Redirect(w, r, "/members/plan", http.StatusSeeOther)
		return
	}
	app.Session.Put(r.Context(), "user", u)
	// redirect
	app.Session.Put(r.Context(), "flash", "subscribed")
	http.Redirect(w, r, "/members/plans", http.StatusSeeOther)
}

func (app *Config) generateManual(u data.User, plan *data.Plan) *gofpdf.Fpdf {
	pdf := gofpdf.New("P", "mm", "Letter", "")
	pdf.SetMargins(10, 13, 10)

	importer := gofpdi.NewImporter()

	time.Sleep(5 * time.Second)

	t := importer.ImportPage(pdf, "./pdf/manual.pdf", 1, "/MediaBox")
	pdf.AddPage()

	importer.UseImportedTemplate(pdf, t, 0, 0, 215.9, 0)
	// where in the page
	pdf.SetX(75)
	pdf.SetY(150)
	pdf.SetFont("Arial", "", 12)
	pdf.MultiCell(0, 4, fmt.Sprintf("%s %s", u.FirstName, u.LastName), "", "C", false)
	pdf.Ln(5) // print blank lines
	pdf.MultiCell(0, 4, fmt.Sprintf("%s User Guide", plan.PlanName), "", "C", false)
	return pdf
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
