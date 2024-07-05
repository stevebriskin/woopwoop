package woop

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
	"github.com/edaniels/golog"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.viam.com/rdk/components/board"
	"go.viam.com/rdk/robot/client"
	"go.viam.com/utils/rpc"
)

func makeLogger(name string) golog.Logger {
	cfg := zap.Config{
		Level:    zap.NewAtomicLevelAt(zap.InfoLevel),
		Encoding: "console",
		EncoderConfig: zapcore.EncoderConfig{
			TimeKey:        "ts",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller",
			FunctionKey:    zapcore.OmitKey,
			MessageKey:     "msg",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.CapitalLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.StringDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		},
		DisableStacktrace: true,
		OutputPaths:       []string{"stdout"},
		ErrorOutputPaths:  []string{"stderr"},
	}

	logger, _ := cfg.Build()
	return logger.Sugar().Named(name)
}

func RobotClient(ctx context.Context, machineUri string, apiKey string, apiKeyId string, logger *zap.SugaredLogger, numRetries int) (*client.RobotClient, error) {
	var err error = nil
	logger.Info("Connecting to 'smart' machine: ", machineUri)

	for i := 0; i < numRetries; i++ {
		var robot *client.RobotClient
		ctx, cancelfx := context.WithTimeout(ctx, 20*time.Second)
		defer cancelfx()

		robot, err = client.New(
			ctx,
			machineUri,
			logger,
			client.WithDisableSessions(),
			client.WithReconnectEvery(0),
			client.WithCheckConnectedEvery(0),
			client.WithRefreshEvery(0),
			client.WithDialOptions(rpc.WithEntityCredentials(
				apiKeyId,
				rpc.Credentials{
					Type:    rpc.CredentialsTypeAPIKey,
					Payload: apiKey,
				})),
		)

		if err == nil {
			logger.Info("Connected to machine.")
			return robot, nil
		}

		logger.Info("Connection to machine failed, sleep and try again.", err)
		time.Sleep(time.Duration(i) * time.Second)
	}
	logger.Warn("Failed to connect to machine.")

	return nil, err
}

func init() {
	functions.HTTP("woop", woop)
}

// helloHTTP is an HTTP Cloud Function with a request parameter.
func woop(w http.ResponseWriter, r *http.Request) {
	if len(r.URL.Query()) == 0 {
		doGCPAlert(w, r)
	} else if r.URL.Query().Get("v") == "3" {
		doV3(w, r)
	} else {
		doBasicQueryParam(w, r)
	}
}

// TODO: original code, needs to be generalized and combined with the query param one. Mostly here for history.
// This is meant for a GCP alert webhook
func doGCPAlert(w http.ResponseWriter, r *http.Request) {
	api_key := os.Getenv("api_key")
	api_key_id := os.Getenv("api_key_id")
	logger := makeLogger("client")

	var alert map[string]any
	if err := json.NewDecoder(r.Body).Decode(&alert); err != nil {
		logger.Error("failed to decode body.", err)
		return
	}
	logger.Info("Received Body:", alert)

	incident := alert["incident"].(map[string]any)
	summary := incident["summary"].(string)
	state := incident["state"].(string)

	logger.Info("Recieved an alert with summary: ", summary)

	fmt.Fprint(w, "connecting")
	ctx := context.Background()

	// TODO: don't hardcode
	robot, err := RobotClient(ctx, "woop-woop-main.6xs7zv3bxz.viam.cloud", api_key, api_key_id, logger, 5)
	if err != nil {
		logger.Error("Failed to connect to robot.", err)
		return
	}
	defer robot.Close(ctx)

	fmt.Fprint(w, "getting board")
	esp, err := board.FromRobot(robot, "board")
	if err != nil {
		logger.Error("No board found.", err)
		return
	}
	defer esp.Close(ctx)

	pin, err := esp.GPIOPinByName("12")
	if err != nil {
		logger.Error("No gpio pin found.", err)
		return
	}

	pinValue := strings.EqualFold(state, "open")
	logger.Info("Setting pin to ", pinValue)
	err = pin.Set(ctx, pinValue, nil)
	if err != nil {
		logger.Error("Couldn't set pin value", err)
		return
	}
}

// expect strobe, buzzer, and woop number as query parameters.
// example: `curl -X GET 'https://us-central1-.cloudfunctions.net/woop?woop=3&secret=xyz&strobe=off&buzzer=on'`
func doBasicQueryParam(w http.ResponseWriter, r *http.Request) {
	api_key := os.Getenv("api_key")
	api_key_id := os.Getenv("api_key_id")
	uri_suffix := os.Getenv("uri_suffix")
	secret := os.Getenv("secret")
	logger := makeLogger("client")

	// not good, but good enough for current usage
	passed_secret := r.URL.Query().Get("secret")

	if secret != passed_secret {
		logger.Error("Wrong secret")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// don't want to commit full URIs...the suffix with the location is an env variable
	woop_num := r.URL.Query().Get("woop")
	robot_uri_full := fmt.Sprintf("woopwoop%s-main.%s", woop_num, uri_suffix)

	logger.Info("Connecting to ", robot_uri_full)
	ctx := context.Background()
	robot, err := RobotClient(ctx, robot_uri_full, api_key, api_key_id, logger, 5)
	if err != nil {
		logger.Error("Failed to connect to robot.", err)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	defer robot.Close(ctx)

	esp, err := board.FromRobot(robot, "board")
	if err != nil {
		logger.Error("No board found.", err)
		w.WriteHeader(http.StatusExpectationFailed)
		return
	}
	defer esp.Close(ctx)

	strobePin, err := esp.GPIOPinByName("12")
	if err != nil {
		logger.Error("Strobe gpio pin 12 not found.", err)
		w.WriteHeader(http.StatusExpectationFailed)
		return
	}

	buzzerPin, err := esp.GPIOPinByName("14")
	if err != nil {
		logger.Error("Buzzer gpio pin 14 not found.", err)
		w.WriteHeader(http.StatusExpectationFailed)
		return
	}

	strobeValue := strings.EqualFold(r.URL.Query().Get("strobe"), "on")
	buzzerValue := strings.EqualFold(r.URL.Query().Get("buzzer"), "on")
	logger.Infof("Request: Strobe: %v, Buzzer: %v", strobeValue, buzzerValue)

	logger.Info("Setting strobe pin to ", strobeValue)
	err = strobePin.Set(ctx, strobeValue, nil)
	if err != nil {
		logger.Error("Couldn't set pin value", err)
		w.WriteHeader(http.StatusRequestTimeout)
		return
	}

	logger.Info("Setting buzzer pin to ", buzzerValue)
	err = buzzerPin.Set(ctx, buzzerValue, nil)
	if err != nil {
		logger.Error("Couldn't set pin value", err)
		w.WriteHeader(http.StatusRequestTimeout)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func doV3(w http.ResponseWriter, r *http.Request) {
	api_key := os.Getenv("api_key")
	api_key_id := os.Getenv("api_key_id")
	uri_suffix := os.Getenv("uri_suffix")
	secret := os.Getenv("secret")
	logger := makeLogger("client")

	// not good, but good enough for current usage
	passed_secret := r.URL.Query().Get("secret")

	if secret != passed_secret {
		logger.Error("Wrong secret")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// don't want to commit full URIs...the suffix with the location is an env variable
	woop_num := r.URL.Query().Get("woop")
	robot_uri_full := fmt.Sprintf("woopwoop%s-main.%s", woop_num, uri_suffix)

	logger.Info("Connecting to ", robot_uri_full)
	ctx := context.Background()
	robot, err := RobotClient(ctx, robot_uri_full, api_key, api_key_id, logger, 5)
	if err != nil {
		logger.Error("Failed to connect to robot.", err)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	defer robot.Close(ctx)

	esp, err := board.FromRobot(robot, "board")
	if err != nil {
		logger.Error("No board found.", err)
		w.WriteHeader(http.StatusExpectationFailed)
		return
	}
	defer esp.Close(ctx)

	redPin, err := esp.GPIOPinByName("19")
	if err != nil {
		logger.Error("Red gpio pin 19 not found.", err)
		w.WriteHeader(http.StatusExpectationFailed)
		return
	}

	greenPin, err := esp.GPIOPinByName("18")
	if err != nil {
		logger.Error("Green gpio pin 18 not found.", err)
		w.WriteHeader(http.StatusExpectationFailed)
		return
	}

	bluePin, err := esp.GPIOPinByName("21")
	if err != nil {
		logger.Error("Blue gpio pin 21 not found.", err)
		w.WriteHeader(http.StatusExpectationFailed)
		return
	}

	buzzerPin, err := esp.GPIOPinByName("5")
	if err != nil {
		logger.Error("Buzzer gpio pin 14 not found.", err)
		w.WriteHeader(http.StatusExpectationFailed)
		return
	}

	var request map[string]any
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		logger.Error("failed to decode body.", err)
		return
	}
	logger.Info("Received Body:", request)

	redraw, rok := request["red"]
	greenraw, gok := request["green"]
	blueraw, bok := request["blue"]
	buzzerraw, buzzok := request["buzzer"]

	if rok {
		red := redraw.(map[string]any)
		err = multierr.Combine(redPin.SetPWMFreq(ctx, uint(red["freq"].(float64)), nil), redPin.SetPWM(ctx, red["duty"].(float64), nil))
		if err != nil {
			logger.Error("Couldn't set red pin value", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	if gok {
		green := greenraw.(map[string]any)
		err = multierr.Combine(greenPin.SetPWMFreq(ctx, uint(green["freq"].(float64)), nil), greenPin.SetPWM(ctx, green["duty"].(float64), nil))
		if err != nil {
			logger.Error("Couldn't set green pin value", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	if bok {
		blue := blueraw.(map[string]any)
		err = multierr.Combine(bluePin.SetPWMFreq(ctx, uint(blue["freq"].(float64)), nil), bluePin.SetPWM(ctx, blue["duty"].(float64), nil))
		if err != nil {
			logger.Error("Couldn't set blue pin value", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	if buzzok {
		buzzer := buzzerraw.(bool)
		err = buzzerPin.Set(ctx, buzzer, nil)
		if err != nil {
			logger.Error("Couldn't set pin value", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}
