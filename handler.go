package storageapi

import (
	"encoding/json"
	"net/http"

	"github.com/kiwiidb/bliksem-library/authentication"
	"github.com/kiwiidb/bliksem-library/vouchertemplating"
	"github.com/koding/multiconfig"
	"github.com/sirupsen/logrus"
)

var vt *vouchertemplating.VoucherTemplater

//Request holds the codes and the name of the template
//(based on value of the vouchers)
type Request struct {
	Codes          []string
	TemplateName   string
	CollectionName string
}

//Response kendet
type Response struct {
	URL string
}

func init() {
	vt = &vouchertemplating.VoucherTemplater{}
	m := multiconfig.EnvironmentLoader{}
	err := m.Load(vt)
	if err != nil {
		logrus.Fatal(err)
	}
	m.PrintEnvs(vt)
	logrus.Info(vt.FirebaseAdminCredentials)
	vt.InitFirebase()
}

//StoreVoucherHandler puts vouchers in firebase storage and returns signed url
func StoreVoucherHandler(w http.ResponseWriter, r *http.Request) {
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
	req := Request{}
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		logrus.Error(err.Error())
		http.Error(w, "Something wrong", http.StatusInternalServerError)
		return
	}
	err = vt.DownloadTemplate(req.TemplateName)
	if err != nil {
		logrus.Error(err.Error())
		http.Error(w, "Something wrong", http.StatusInternalServerError)
		return
	}
	url, err := vt.CreateAndUploadZipFromCodes(req.Codes, req.CollectionName)
	if err != nil {
		logrus.Error(err.Error())
		http.Error(w, "Something wrong", http.StatusInternalServerError)
		return
	}
	resp, err := json.Marshal(Response{URL: url})
	if err != nil {
		logrus.Error(err.Error())
		http.Error(w, "Something wrong", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(resp)
}
