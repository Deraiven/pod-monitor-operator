package main

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"time"
)

func main() {
	// 这是从实际 Linkerd Secret 中提取的 crt.pem 的 base64 编码数据
	certDataBase64 := "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUJzekNDQVZpZ0F3SUJBZ0lRS3VIOVl0Y3BnZ1pxZFZuYTFBOXRUekFLQmdncWhrak9QUVFEQWpBbE1TTXcKSVFZRFZRUURFeHB5YjI5MExteHBibXRsY21RdVkyeDFjM1JsY2k1c2IyTmhiREFlRncweU5UQTRNRFl5TXpFeQpNalphRncwek5UQTRNRFF5TXpFeU1qWmFNQ2t4SnpBbEJnTlZCQU1USG1sa1pXNTBhWFI1TG14cGJtdGxjbVF1ClkyeDFjM1JsY2k1c2IyTmhiREJaTUJNR0J5cUdTTTQ5QWdFR0NDcUdTTTQ5QXdFSEEwSUFCTGpUR0JlbVdLUlEKdGFWRS9CT2lTbmd4cUt6SS9pbFRLRFlHbU1peFpuVGtMeXhTUHd5VHM4NkVrS0owbXpONkRLSThkSUQ0VzJsagpudDBKcnQ1Nmd0Q2paakJrTUE0R0ExVWREd0VCL3dRRUF3SUJCakFTQmdOVkhSTUJBZjhFQ0RBR0FRSC9BZ0VBCk1CMEdBMVVkRGdRV0JCVEtjRGZwQ2ZzSHRLOGU0TisxOS8zUjRlNjFmekFmQmdOVkhTTUVHREFXZ0JSbE5zclgKeFYrbWNaMXltaWFOeVdCNFoycU82akFLQmdncWhrak9QUVFEQWdOSkFEQkdBaUVBOHppUVVrQkJVaWR4bjZBdgpNL1piVkFkODl5OWk5U2NRNXNEWTQ4dTRVanNDSVFDR3lKYnBEV1piWmxkMGNDK0c5RnN3bExUenAyZG4vZGpTCjhnRUdPUXNydGc9PQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0t"

	// 解码 base64
	certData, err := base64.StdEncoding.DecodeString(certDataBase64)
	if err != nil {
		fmt.Printf("Failed to decode base64: %v\n", err)
		return
	}

	// 解析 PEM
	block, _ := pem.Decode(certData)
	if block == nil {
		fmt.Println("Failed to parse PEM block")
		return
	}

	// 解析证书
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		fmt.Printf("Failed to parse certificate: %v\n", err)
		return
	}

	// 打印证书信息
	fmt.Printf("Certificate Subject: %s\n", cert.Subject)
	fmt.Printf("Certificate Issuer: %s\n", cert.Issuer)
	fmt.Printf("Not Before: %s\n", cert.NotBefore)
	fmt.Printf("Not After: %s\n", cert.NotAfter)

	// 计算剩余天数
	now := time.Now()
	daysUntilExpiration := cert.NotAfter.Sub(now).Hours() / 24
	fmt.Printf("Days until expiration: %.2f\n", daysUntilExpiration)

	// 检查是否是 CA 证书
	if cert.IsCA {
		fmt.Println("This is a CA certificate")
	}
}
