package broadcast

import (
	"bufio"
	"bytes"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/gommon/log"
	"github.com/sirupsen/logrus"
)

type LoggerConfig struct {
	IncludeRequestBodies  bool
	IncludeResponseBodies bool
}

type logrusLogger struct {
	*logrus.Logger
}

func (l logrusLogger) Level() log.Lvl {
	switch l.Logger.Level {
	case logrus.DebugLevel:
		return log.DEBUG
	case logrus.WarnLevel:
		return log.WARN
	case logrus.ErrorLevel:
		return log.ERROR
	case logrus.InfoLevel:
		return log.INFO
	default:
		l.Panic("Invalid level")
	}

	return log.OFF
}

// SetHeader is a stub to satisfy the logrusLogger interface
// It's controlled by logrus
func (l logrusLogger) SetHeader(_ string) {}

func (l logrusLogger) SetPrefix(_ string) {}

func (l logrusLogger) Prefix() string { return "" }

func (l logrusLogger) SetLevel(lvl log.Lvl) {
	switch lvl {
	case log.DEBUG:
		logrus.SetLevel(logrus.DebugLevel)
	case log.WARN:
		logrus.SetLevel(logrus.WarnLevel)
	case log.ERROR:
		logrus.SetLevel(logrus.ErrorLevel)
	case log.INFO:
		logrus.SetLevel(logrus.InfoLevel)
	default:
		l.Panic("Invalid level")
	}
}

func (l logrusLogger) Output() io.Writer {
	return l.Out
}

func (l logrusLogger) SetOutput(w io.Writer) {
	logrus.SetOutput(w)
}

func (l logrusLogger) Printj(j log.JSON) {
	logrus.WithFields(logrus.Fields(j)).Print()
}

func (l logrusLogger) Debugj(j log.JSON) {
	logrus.WithFields(logrus.Fields(j)).Debug()
}

func (l logrusLogger) Infoj(j log.JSON) {
	logrus.WithFields(logrus.Fields(j)).Info()
}

func (l logrusLogger) Warnj(j log.JSON) {
	logrus.WithFields(logrus.Fields(j)).Warn()
}

func (l logrusLogger) Errorj(j log.JSON) {
	logrus.WithFields(logrus.Fields(j)).Error()
}

func (l logrusLogger) Fatalj(j log.JSON) {
	logrus.WithFields(logrus.Fields(j)).Fatal()
}

func (l logrusLogger) Panicj(j log.JSON) {
	logrus.WithFields(logrus.Fields(j)).Panic()
}

func logrusMiddlewareHandler(c echo.Context, next echo.HandlerFunc, config LoggerConfig) error {
	start := time.Now()

	// Request
	req := c.Request()
	var reqBody []byte
	if config.IncludeRequestBodies {
		if req.Body != nil { // Read
			reqBody, _ = ioutil.ReadAll(req.Body)
		}
		req.Body = ioutil.NopCloser(bytes.NewBuffer(reqBody)) // Reset
	}

	// Response
	res := c.Response()
	resBody := new(bytes.Buffer)
	if config.IncludeResponseBodies {
		mw := io.MultiWriter(res.Writer, resBody)
		writer := &bodyDumpResponseWriter{Writer: mw, ResponseWriter: c.Response().Writer}
		res.Writer = writer
	}

	var err error
	if err = next(c); err != nil {
		c.Error(err)
	}

	stop := time.Now()
	fieldsMap := map[string]interface{}{
		"time_rfc3339":  time.Now().UTC().Format(time.RFC3339),
		"remote_ip":     c.RealIP(),
		"host":          req.Host,
		"uri":           req.RequestURI,
		"method":        req.Method,
		"path":          getPath(req),
		"referer":       req.Referer(),
		"user_agent":    req.UserAgent(),
		"status":        res.Status,
		"latency":       strconv.FormatInt(stop.Sub(start).Nanoseconds()/1000, 10),
		"latency_human": stop.Sub(start).String(),
		"bytes_in":      getBytesIn(req),
		"bytes_out":     strconv.FormatInt(res.Size, 10),
		"request_id":    getRequestID(req, res),
		"error":         err,
	}

	if config.IncludeRequestBodies {
		fieldsMap["request_body"] = string(reqBody)
	}

	if config.IncludeResponseBodies {
		fieldsMap["response_body"] = resBody.String()
	}

	logrus.WithFields(fieldsMap).Info("handled request")

	return nil
}

func getBytesIn(req *http.Request) string {
	bytesIn := req.Header.Get(echo.HeaderContentLength)
	if bytesIn == "" {
		bytesIn = "0"
	}
	return bytesIn
}

func getPath(req *http.Request) string {
	p := req.URL.Path
	if p == "" {
		p = "/"
	}
	return p
}

func getRequestID(req *http.Request, res *echo.Response) string {
	var id = req.Header.Get(echo.HeaderXRequestID)
	if id == "" {
		id = res.Header().Get(echo.HeaderXRequestID)
	}
	return id
}

func loggerHookWithConfig(config LoggerConfig) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			return logrusMiddlewareHandler(c, next, config)
		}
	}
}

func loggerHook() echo.MiddlewareFunc {
	return loggerHookWithConfig(LoggerConfig{})
}

type bodyDumpResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w *bodyDumpResponseWriter) WriteHeader(code int) {
	w.ResponseWriter.WriteHeader(code)
}

func (w *bodyDumpResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

func (w *bodyDumpResponseWriter) Flush() {
	w.ResponseWriter.(http.Flusher).Flush()
}

func (w *bodyDumpResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.ResponseWriter.(http.Hijacker).Hijack()
}

func (w *bodyDumpResponseWriter) CloseNotify() <-chan bool {
	return w.ResponseWriter.(http.CloseNotifier).CloseNotify()
}
