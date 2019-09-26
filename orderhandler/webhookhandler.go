package orderhandler

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/schema"
	"github.com/kiwiidb/bliksem-library/opennode"
	"github.com/kiwiidb/bliksem-library/tokendb"
	"github.com/kiwiidb/bliksem-library/utils"
	"github.com/kiwiidb/bliksem-library/vouchertemplating"
	"github.com/koding/multiconfig"
	"github.com/sirupsen/logrus"
)

//Order some vouchers
type Order struct {
	Value    int
	Amt      int
	Currency string
	Email    string
}

//WebHookRequestBody to check authenticity of OpenNode request
type WebHookRequestBody struct {
	HashedOrder string `schema:"hashed_order"`
	ID          string `schema:"id"`
	CallBackURL string `schema:"callback_url"`
	SuccesURL   string `schema:"success_url"`
	Status      string `schema:"status"`
	OrderID     string `schema:"order_id"`
	Description string `schema:"description"`
	Price       string `schema:"price"`
	Fee         string `schema:"fee"`
	AutoSettle  string `schema:"auto_settle"`
}

var vt *vouchertemplating.VoucherTemplater
var tdb *tokendb.TokenDB
var on *opennode.OpenNode
var ms *utils.MailSender

func init() {
	//init firebase
	vt = &vouchertemplating.VoucherTemplater{}
	m := multiconfig.EnvironmentLoader{}
	err := m.Load(vt)
	if err != nil {
		logrus.Fatal(err)
	}
	m.PrintEnvs(vt)
	logrus.Info(vt.FirebaseAdminCredentials)
	//err = vt.InitFirebase()
	if err != nil {
		logrus.Fatal(err)
	}

	//init database
	tdb = &tokendb.TokenDB{}
	conf := tokendb.Config{}
	m = multiconfig.EnvironmentLoader{}
	err = m.Load(&conf)
	if err != nil {
		logrus.Fatal(err)
	}
	m.PrintEnvs(conf)
	logrus.Info(conf)
	//err = tdb.Initialize(conf)
	if err != nil {
		logrus.Fatal(err)
	}
	//TODO deftige init voor alle library functions
	on = &opennode.OpenNode{}
	on.APIKey = os.Getenv("OPENNODE_APIKEY")
	ms = &utils.MailSender{}
	//ms.Init()

}

//WebhookHandler to be called by Opennode
func WebhookHandler(w http.ResponseWriter, r *http.Request) {
	decoder := schema.NewDecoder()
	order := Order{}
	whrb := WebHookRequestBody{}
	err := decoder.Decode(&order, r.URL.Query())
	if err != nil {
		logrus.Error(err)
		http.Error(w, "something wrong decoding", http.StatusBadRequest)
		return
	}
	err = r.ParseForm()
	if err != nil {
		logrus.Error(err)
		http.Error(w, "something wrong decoding", http.StatusBadRequest)
		return
	}
	err = decoder.Decode(&whrb, r.PostForm)
	if err != nil {
		logrus.Error(err)
		http.Error(w, "something wrong decoding", http.StatusBadRequest)
		return
	}
	if !utils.ValidMAC([]byte(whrb.ID), whrb.HashedOrder, []byte(on.APIKey)) {
		//invalid
		logrus.Error(fmt.Errorf("webhook called with wrong token %v %v", whrb, r))
		http.Error(w, "something wrong decoding", http.StatusBadRequest)
		return
	}
	_, err = tdb.CreateNewBatchOfTokens(whrb.ID, order.Amt, order.Value, order.Currency, true) //online sold vouchers are always already on
	if err != nil {
		logrus.Error(err)
		http.Error(w, "something wrong decoding", http.StatusBadRequest)
		return
	}
	codes, err := tdb.GetAllTokensInCollection(whrb.ID)
	if err != nil {
		logrus.Error(err)
		http.Error(w, "something wrong decoding", http.StatusBadRequest)
		return
	}
	formattedCodes := []string{}
	formatString := "https://flitz-api-now.kwintendebacker.now.sh/lnurl-primary/%s/%s"
	for _, code := range codes {
		toAppend, err := utils.EncodeToLNURL(fmt.Sprintf(formatString, whrb.ID, code.ID))
		if err != nil {
			logrus.Error(err)
			http.Error(w, "something wrong decoding", http.StatusBadRequest)
			return
		}
		formattedCodes = append(formattedCodes, toAppend)
	}

	var storageURL string
	var localFile string
	if order.Amt > 1 {
		templateFilename := fmt.Sprintf("voucher_%d_%s.png", order.Value, order.Currency)
		err = vt.DownloadTemplate(templateFilename)
		if err != nil {
			logrus.Error(err)
			http.Error(w, "something wrong decoding", http.StatusBadRequest)
			return
		}
		storageURL, err = vt.CreateAndUploadZipFromCodes(formattedCodes, whrb.ID)
		if err != nil {
			logrus.Error(err)
			http.Error(w, "something wrong decoding", http.StatusBadRequest)
			return
		}
		localFile = "/tmp/voucher.zip"
	} else {
		templateFilename := fmt.Sprintf("voucher_custom.png")
		err = vt.DownloadTemplate(templateFilename)
		if err != nil {
			logrus.Error(err)
			http.Error(w, "something wrong decoding", http.StatusBadRequest)
			return
		}
		storageURL, err = vt.CreateAndUploadSingleVoucher(formattedCodes[0])
		if err != nil {
			logrus.Error(err)
			http.Error(w, "something wrong decoding", http.StatusBadRequest)
			return
		}
		localFile = "/tmp/voucher.png"
	}
	err = ms.DownloadVoucher(storageURL, localFile)
	if err != nil {
		logrus.Error(err)
		http.Error(w, "something wrong ", http.StatusInternalServerError)
		return
	}
	emailBody := ""
	if len(formattedCodes) == 1 {
		emailBody = strings.Replace(singleEmailBodyTemplate, "TOREPLACE", formattedCodes[0], 1)
		emailBody = strings.Replace(emailBody, "CURRENCY", order.Currency, 1)
		emailBody = strings.Replace(emailBody, "AMOUNT", string(order.Value), 1)
	} else {
		emailBody = multiEmailBody
	}
	emailBody, err = createEmailBody(order, formattedCodes)
	if err != nil {
		logrus.Error(err)
		http.Error(w, "something wrong ", http.StatusInternalServerError)
		return
	}
	err = ms.SendMail(order.Email, emailBody, localFile)
	if err != nil {
		logrus.Error(err)
		http.Error(w, "something wrong ", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	return
}

func createEmailBody(order Order, formattedCodes []string) (string, error) {
	type EmailBodyInfo struct {
		Currency string
		Amount   int
		LNURL    string
	}
	ebi := EmailBodyInfo{
		Currency: order.Currency,
		Amount:   order.Amt,
		LNURL:    formattedCodes[0],
	}
	if len(formattedCodes) > 1 {
		return multiEmailBody, nil
	}
	tmpl, err := template.New("emailbody").Parse(singleEmailBodyTemplate)
	if err != nil {
		return "", err
	}
	var bb bytes.Buffer
	err = tmpl.Execute(&bb, ebi)
	if err != nil {
		return "", err
	}
	return bb.String(), nil
}

var singleEmailBodyTemplate = `
<DOCTYPE html>
<html style="font:Arial;">
<body style="text-align:center; ">
<h2 style="font:Arial;">Hello there!</h2>
<p style="font:Arial;">
You have received a Flitz voucher for {{.Currency}} {{.Amount}}.
Use your favourite LNURL-enabled wallet to redeem it.
<br>
You can scan the QR code or click here:
</p>

<p style="font:Arial;">
<a class= "button" href="lightning:{{.LNURL}}">Redeem in Wallet</a>
</p>

<p style="font:Arial;">
Kind regards,
The Flitz team.
</p>
</body>
</html>
<style>
    .button {
  font: bold 11px Arial;
  text-decoration: none;
  background-color: #2c3e50;
  color: mediumspringgreen;
  padding: 2px 6px 2px 6px;
  border-top: 1px solid ;
  border-right: 1px solid ;
  border-bottom: 1px solid ;
  border-left: 1px solid ;
  border-radius: 25px;
  border-color: mediumspringgreen;
}
</style>
`

var multiEmailBody = `
Hello there!
You have received a Flitz voucher.
Use your favourite LNURL-enabled wallet to redeem it.
Kind regards,
The Flitz team.
`
