package log

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

type Logger interface {
	SetLevel(level log.Level)

	BuildContextDataAndSetValue(contextId string) (ctx context.Context)
	AppendContextDataAndSetValue(r *http.Request, contextId string) *http.Request
	SetContextDataAndSetValue(r *http.Request, data map[string]string, contextId string) *http.Request

	CreateResponseWrapper(rw http.ResponseWriter) *LoggingResponseWriter

	GetEntry() *log.Entry

	Infof(ctx context.Context, message string, args ...interface{})
	Errorf(ctx context.Context, message string, args ...interface{})
	Warnf(ctx context.Context, message string, args ...interface{})
	Debugf(ctx context.Context, message string, args ...interface{})
	Fatalf(ctx context.Context, message string, args ...interface{})
	Info(ctx context.Context, args ...interface{})
	Error(ctx context.Context, args ...interface{})
	Warn(ctx context.Context, args ...interface{})
	Debug(ctx context.Context, args ...interface{})
	Fatal(ctx context.Context, args ...interface{})

	InfoMap(ctx context.Context, dataMap map[string]interface{}, args ...interface{})

	LogRequest(ctx context.Context, r *http.Request)
	LogResponse(ctx context.Context, rw *LoggingResponseWriter)
}

// safe typing https://golang.org/pkg/context/#WithValue
type contextDataMapKeyType string

// add key here for future request based value
var (
	// ContextDataMapKey is a key for data map that contain values
	ContextDataMapKey contextDataMapKeyType = "value"

	// context key data added to map
	ContextIdKey    = "context_id"
	PathKey         = "url_path"
	RequestKey      = "request"
	ResponseKey     = "response"
	ResponseCodeKey = "response_code"
)

type Log struct {
	entry *log.Entry
}

type LogParams struct {
	fields log.Fields
}

// context key data added to map
type contextData struct {
	contextId string
}

const (
	maximumCallerDepth int = 25
	knownLogFrames     int = 3
)

var (
	minimumCallerDepth = 1

	// Used for caller information initialisation
	callerInitOnce sync.Once

	// qualified package name, cached at first use
	thisPackageName string
)

func NewLogger(service string) Logger {
	logger := log.New()

	logger.SetFormatter(&log.JSONFormatter{
		TimestampFormat: time.RFC3339Nano,
	})
	entry := log.NewEntry(logger)
	entry = entry.WithField("service", service)
	return &Log{entry}
}

func NewLoggerWithLevel(service string, level log.Level) Logger {
	logger := log.New()

	logger.SetFormatter(&log.JSONFormatter{
		TimestampFormat: time.RFC3339Nano,
	})
	logger.SetLevel(level)
	entry := log.NewEntry(logger)
	entry = entry.WithField("service", service)
	return &Log{entry}
}

func (l *Log) SetLevel(level log.Level) {
	l.entry.Logger.SetLevel(level)
}

func (l *Log) getContextData(ctx context.Context) *contextData {
	dataMap := ctx.Value(ContextDataMapKey)
	var result *contextData

	if dataMap != nil {
		if data, ok := dataMap.(map[string]string); ok {
			result = &contextData{
				contextId: data[ContextIdKey],
			}
		}
	}

	return result
}

func (l *Log) BuildContextDataAndSetValue(contextId string) (ctx context.Context) {
	data := make(map[string]string, 0)
	data[ContextIdKey] = contextId

	ctx = context.WithValue(context.Background(), ContextDataMapKey, data)

	return ctx
}

func (l *Log) AppendContextDataAndSetValue(r *http.Request, contextId string) *http.Request {
	data := make(map[string]string, 0)
	data[ContextIdKey] = contextId

	ctx := context.WithValue(r.Context(), ContextDataMapKey, data)

	return r.WithContext(ctx)
}

func (l *Log) SetContextDataAndSetValue(r *http.Request, data map[string]string, contextId string) *http.Request {
	if data == nil {
		data = make(map[string]string, 0)
	}
	data[ContextIdKey] = contextId

	ctx := context.WithValue(r.Context(), ContextDataMapKey, data)

	return r.WithContext(ctx)
}

func (l *Log) CreateResponseWrapper(rw http.ResponseWriter) *LoggingResponseWriter {
	return &LoggingResponseWriter{
		ResponseWriter: rw,
	}
}

func (l *Log) GetEntry() *log.Entry {
	return l.entry
}

func (l *Log) Infof(ctx context.Context, message string, args ...interface{}) {
	lp := LogParams{fields: log.Fields{}}
	lp.setCallStackTrace(log.InfoLevel)
	lp.injectContextDataMap(ctx)
	l.entry.WithFields(lp.fields).Infof(message, args...)
}

func (l *Log) Warnf(ctx context.Context, message string, args ...interface{}) {
	lp := LogParams{fields: log.Fields{}}
	lp.setCallStackTrace(log.WarnLevel)
	lp.injectContextDataMap(ctx)
	l.entry.WithFields(lp.fields).Warningf(message, args...)
}

func (l *Log) Errorf(ctx context.Context, message string, args ...interface{}) {
	lp := LogParams{fields: log.Fields{}}
	lp.setCallStackTrace(log.ErrorLevel)
	lp.injectContextDataMap(ctx)
	l.entry.WithFields(lp.fields).Errorf(message, args...)
}

func (l *Log) Debugf(ctx context.Context, message string, args ...interface{}) {
	lp := LogParams{fields: log.Fields{}}
	lp.setCallStackTrace(log.DebugLevel)
	lp.injectContextDataMap(ctx)
	l.entry.WithFields(lp.fields).Debugf(message, args...)
}

func (l *Log) Fatalf(ctx context.Context, message string, args ...interface{}) {
	lp := LogParams{fields: log.Fields{}}
	lp.setCallStackTrace(log.FatalLevel)
	lp.injectContextDataMap(ctx)
	l.entry.WithFields(lp.fields).Fatalf(message, args...)
}

func (l *Log) Info(ctx context.Context, args ...interface{}) {
	lp := LogParams{fields: log.Fields{}}
	lp.setCallStackTrace(log.InfoLevel)
	lp.injectContextDataMap(ctx)
	l.entry.WithFields(lp.fields).Info(args...)
}

func (l *Log) Warn(ctx context.Context, args ...interface{}) {
	lp := LogParams{fields: log.Fields{}}
	lp.setCallStackTrace(log.WarnLevel)
	lp.injectContextDataMap(ctx)
	l.entry.WithFields(lp.fields).Warning(args...)
}

func (l *Log) Error(ctx context.Context, args ...interface{}) {
	lp := LogParams{fields: log.Fields{}}
	lp.setCallStackTrace(log.ErrorLevel)
	lp.injectContextDataMap(ctx)
	l.entry.WithFields(lp.fields).Error(args...)
}

func (l *Log) Debug(ctx context.Context, args ...interface{}) {
	lp := LogParams{fields: log.Fields{}}
	lp.setCallStackTrace(log.DebugLevel)
	lp.injectContextDataMap(ctx)
	l.entry.WithFields(lp.fields).Debug(args...)
}

func (l *Log) Fatal(ctx context.Context, args ...interface{}) {
	lp := LogParams{fields: log.Fields{}}
	lp.setCallStackTrace(log.FatalLevel)
	lp.injectContextDataMap(ctx)
	l.entry.WithFields(lp.fields).Fatal(args...)
}

func (l *Log) InfoMap(ctx context.Context, dataMap map[string]interface{}, args ...interface{}) {
	lp := LogParams{fields: log.Fields{}}
	lp.setCallStackTrace(log.InfoLevel)
	lp.injectContextDataMap(ctx)

	if dataMap != nil {
		for key, value := range dataMap {
			lp.fields[key] = value
		}
	}

	l.entry.WithFields(lp.fields).Info(args...)
}

func (l *Log) LogRequest(ctx context.Context, r *http.Request) {
	lp := LogParams{fields: log.Fields{}}
	lp.setCallStackTrace(log.InfoLevel)
	lp.injectContextDataMap(ctx).injectURLPath(ctx, r).injectRequestBody(ctx, r)
	l.entry.WithFields(lp.fields).Info("Request Body")
}

func (l *Log) LogResponse(ctx context.Context, rw *LoggingResponseWriter) {
	lp := LogParams{fields: log.Fields{}}
	lp.setCallStackTrace(log.InfoLevel)
	lp.injectContextDataMap(ctx).injectResponseBody(ctx, rw)
	l.entry.WithFields(lp.fields).Info("Response Body")
}

func (lp *LogParams) setCallStackTrace(logLevel log.Level) {
	if logLevel <= log.ErrorLevel {
		lp.setCaller(getCaller())
	}
}

func (lp *LogParams) setCaller(caller *runtime.Frame) {
	if caller == nil {
		return
	}

	funcVal := caller.Function
	fileVal := fmt.Sprintf("%s:%d", caller.File, caller.Line)
	if funcVal != "" {
		lp.fields["func"] = funcVal
	}
	if fileVal != "" {
		lp.fields["file"] = fileVal
	}
}

func (lp *LogParams) injectContextDataMap(ctx context.Context) *LogParams {
	dataMap := ctx.Value(ContextDataMapKey)

	if dataMap != nil {
		if data, ok := dataMap.(map[string]string); ok {
			for key, value := range data {
				lp.fields[key] = value
			}
		}
	}

	return lp
}

func (lp *LogParams) injectURLPath(ctx context.Context, r *http.Request) *LogParams {
	lp.fields[PathKey] = r.Host + r.URL.Path
	return lp
}

func (lp *LogParams) injectRequestBody(ctx context.Context, r *http.Request) *LogParams {
	buf, _ := ioutil.ReadAll(r.Body)
	r.Body.Close()
	r.Body = ioutil.NopCloser(bytes.NewBuffer(buf))

	lp.fields[RequestKey] = fmt.Sprintf("%q", r.Body)
	return lp
}

func (lp *LogParams) injectResponseBody(ctx context.Context, rw *LoggingResponseWriter) *LogParams {
	lp.fields[ResponseCodeKey] = rw.Status
	lp.fields[ResponseKey] = rw.Body
	return lp
}

type LoggingResponseWriter struct {
	Status int
	Body   string
	http.ResponseWriter
}

func (w *LoggingResponseWriter) WriteHeader(code int) {
	w.Status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *LoggingResponseWriter) Write(body []byte) (int, error) {
	w.Body = string(body)
	return w.ResponseWriter.Write(body)
}

// getCaller retrieves the name of the first non this package calling function
func getCaller() *runtime.Frame {

	// cache this package's fully-qualified name
	callerInitOnce.Do(func() {
		// get log package name (this package)
		pcs := make([]uintptr, 2)
		frames := runtime.CallersFrames(pcs[:runtime.Callers(1, pcs)])
		frame, _ := frames.Next()
		thisPackageName = getPackageName(frame.Function)

		// now that we have the cache, we can skip a minimum count of known functions
		// XXX this is dubious, the number of frames may vary
		minimumCallerDepth = knownLogFrames
	})

	// Restrict the lookback frames to avoid runaway lookups
	pcs := make([]uintptr, maximumCallerDepth)
	depth := runtime.Callers(minimumCallerDepth, pcs)
	frames := runtime.CallersFrames(pcs[:depth])

	for f, again := frames.Next(); again; f, again = frames.Next() {
		pkg := getPackageName(f.Function)

		// If the caller isn't part of this package, we're done
		if pkg != thisPackageName {
			return &f
		}
	}

	// if we got here, we failed to find the caller's context
	return nil
}

// getPackageName reduces a fully qualified function name to the package name
func getPackageName(f string) string {
	for {
		lastPeriod := strings.LastIndex(f, ".")
		lastSlash := strings.LastIndex(f, "/")
		if lastPeriod > lastSlash {
			f = f[:lastPeriod]
		} else {
			break
		}
	}

	return f
}
