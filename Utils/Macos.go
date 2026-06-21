//go:build darwin
// +build darwin

package Utils

import "errors"

func InstallCert(certName string) error {

	return errors.New("不支持Macos系统")
}

func SetSystemProxy(proxy string) error {

	return errors.New("不支持Macos系统")
}
