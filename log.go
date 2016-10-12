package main

import "github.com/reconquest/hierr-go"

func fatalf(format string, values ...interface{}) {
	logger.Fatalf(format, values...)
}

func errorf(format string, values ...interface{}) {
	logger.Errorf(format, values...)
}

func warningf(format string, values ...interface{}) {
	logger.Warningf(format, values...)
}

func infof(format string, values ...interface{}) {
	logger.Infof(format, values...)
}

func debugf(format string, values ...interface{}) {
	logger.Debugf(format, values...)
}

func tracef(format string, values ...interface{}) {
	logger.Tracef(format, values...)
}

func debugln(value interface{}) {
	logger.Debug(value)
}

func infoln(value interface{}) {
	logger.Info(value)
}

func fatalln(value interface{}) {
	logger.Fatal(value)
}

func errorln(value interface{}) {
	logger.Error(value)
}

func fatalh(err error, format string, args ...interface{}) {
	logger.Fatal(hierr.Errorf(err, format, args...))
}

func warningh(err error, format string, args ...interface{}) {
	logger.Warning(hierr.Errorf(err, format, args...))
}

func errorh(err error, format string, args ...interface{}) {
	logger.Error(hierr.Errorf(err, format, args...))
}
