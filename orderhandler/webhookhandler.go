package orderhandler

import (
	"fmt"
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
	err = vt.InitFirebase()
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
	err = tdb.Initialize(conf)
	if err != nil {
		logrus.Fatal(err)
	}
	//TODO deftige init voor alle library functions
	on = &opennode.OpenNode{}
	on.APIKey = os.Getenv("OPENNODE_APIKEY")
	ms = &utils.MailSender{}
	ms.Init()

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
		emailBody = strings.Replace(singleEmailBody, "TOREPLACE", formattedCodes[0], 1)
	} else {
		emailBody = multiEmailBody
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

var singleEmailBody = `
<DOCTYPE html>
<html>
<body style="text-align:center">
<h2>Hello there!</h2>
<p>
You have received a Flitz voucher for CURRENCY AMOUNT.
Use your favourite LNURL-enabled wallet to redeem it.
<br>
You can scan the QR code or click here:
</p>

<p>
<a href="lightning:TOREPLACE">Redeem in Wallet</a>
</p>

<p>
Kind regards,
The Flitz team.
</p>
</body>
</html>
`
var multiEmailBody = `
Hello there!
You have received a Flitz voucher.
Use your favourite LNURL-enabled wallet to redeem it.
Kind regards,
The Flitz team.
`
