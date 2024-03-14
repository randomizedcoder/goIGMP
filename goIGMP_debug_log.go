package goIGMP

import "log"

func debugLog(logIt bool, str string) {
	if logIt {
		log.Print(str)
	}
}
