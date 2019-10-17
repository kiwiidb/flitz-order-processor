package adminhandler

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/kiwiidb/bliksem-library/authentication"
	"github.com/kiwiidb/bliksem-library/tokendb"
	"github.com/kiwiidb/bliksem-library/utils"
	"github.com/kiwiidb/bliksem-library/vouchertemplating"
	"github.com/koding/multiconfig"
	"github.com/sirupsen/logrus"
)

var vt *vouchertemplating.VoucherTemplater
var tdb *tokendb.TokenDB

//Response kendet
type Response struct {
	URL string
}

//Order some vouchers
type AdminOrder struct {
	Value     int
	Amt       int
	Currency  string
	BatchName string
}

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
	vt.InitFirebase()

	//init database
	tdb = &tokendb.TokenDB{}
	err = tdb.Initialize()
	logrus.WithError(err).Error("something wrong here")
	if err != nil {
		logrus.Fatal(err)
	}

}

//AuthCreateVoucherHandler puts vouchers in firebase storage and returns signed url
func AuthCreateVoucherHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	//ugly check for options call
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	authenticated, err := authentication.CheckFirebaseAuthentication(vt.FirebaseAdminCredentials, r)
	if !authenticated || err != nil {
		logrus.WithField("request", *r).WithError(err).Info("Unauthenticated request to API")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	reqBytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
	}
	adminOrder := AdminOrder{}
	err = json.Unmarshal(reqBytes, &adminOrder)
	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
	}
	_, err = tdb.CreateNewBatchOfTokens(adminOrder.BatchName, adminOrder.Amt, adminOrder.Value, adminOrder.Currency, true) //online sold vouchers are always already on
	if err != nil {
		logrus.Error(err)
		http.Error(w, "something wrong decoding", http.StatusBadRequest)
		return
	}
	codes, err := tdb.GetAllTokensInCollection(adminOrder.BatchName)
	if err != nil {
		logrus.Error(err)
		http.Error(w, "something wrong decoding", http.StatusBadRequest)
		return
	}
	formattedCodes := []string{}
	//TODO remove hardcoded string!
	formatString := "https://api.flitz.cards/lnurl-primary/%s/%s"
	for _, code := range codes {
		toAppend, err := utils.EncodeToLNURL(fmt.Sprintf(formatString, adminOrder.BatchName, code.ID))
		if err != nil {
			logrus.Error(err)
			http.Error(w, "something wrong decoding", http.StatusBadRequest)
			return
		}
		formattedCodes = append(formattedCodes, toAppend)
	}

	var storageURL string
	if adminOrder.Amt > 1 {
		templateFilename := fmt.Sprintf("voucher_custom.png")
		err = vt.DownloadVoucherTemplateAndLoadInMemory(templateFilename)
		if err != nil {
			logrus.Error(err)
			http.Error(w, "something wrong decoding", http.StatusBadRequest)
			return
		}
		storageURL, err = vt.CreateAndUploadZipFromCodes(formattedCodes, adminOrder.BatchName)
		if err != nil {
			logrus.Error(err)
			http.Error(w, "something wrong decoding", http.StatusBadRequest)
			return
		}
	} else {
		templateFilename := fmt.Sprintf("voucher_custom.png")
		err = vt.DownloadVoucherTemplateAndLoadInMemory(templateFilename)
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
	}

	resp, err := json.Marshal(Response{URL: storageURL})
	if err != nil {
		logrus.Error(err.Error())
		http.Error(w, "Something wrong", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(resp)
}
