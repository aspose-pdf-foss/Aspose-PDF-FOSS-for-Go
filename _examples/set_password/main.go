package main

import (
	"log"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func main() {
	doc, err := pdf.Open("testdata/alfa.pdf")
	if err != nil {
		log.Fatalf("open: %v", err)
	}

	doc.SetPassword("secret", "owner-secret")

	if err := doc.Save("result_files/alfa_encrypted.pdf"); err != nil {
		log.Fatalf("save: %v", err)
	}
	log.Println("saved result_files/alfa_encrypted.pdf (user pw: secret, owner pw: owner-secret)")
}
