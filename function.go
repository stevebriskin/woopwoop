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
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.viam.com/rdk/components/board"
	"go.viam.com/rdk/robot/client"
	"go.viam.com/utils/rpc"
)

/*
type GCPAlertIncident struct {
	Incident_id            string `json:"incident_id"`            //string, generated ID for this incident.
	Scoping_project_id     string `json:"scoping_project_id"`     //string, the project ID that hosts the metrics scope.
	Scoping_project_number int    `json:"scoping_project_number"` //number, the project number of the scoping project.
	Url                    string `json:"url"`                    //string, Google Cloud console URL for this incident.
	Started_at             uint64 `json:"started_at"`             //number, time (in Unix epoch seconds) when the incident was opened.
	Ended_at               uint64 `json:"ended_at"`               //number, time (in Unix epoch seconds) when the incident was closed. Populated only when state is closed.
	State                  string `json:"state"`                  //string, state of the incident: open or closed. If open, then ended_at is null.
	Summary                string `json:"summary"`                //string, generated textual summary of the incident.
	Apigee_url             string `json:"apigee_url"`             //string, Apigee URL for this incident, only for Apigee resource types Environment and Proxy*.
	Observed_value         string `json:"observed_value"`         //string, observed value that triggered/resolved the alert, may be empty if the condition is expired.

	Resource map[string]any `json:"resource"`
	Metric   map[string]any `json:"metric"`

	Policy_name        string         `json:"policy_name"`        // string, display name for the alerting policy.
	Policy_user_labels map[string]any `json:"policy_user_labels"` //"policy_user_labels": object, key-value pairs for any user labels attached to the policy.
	Documentation      map[string]any `json:"documentation"`      //: object, an embedded structure of the form Documentation.
	Condition          map[string]any `json:"condition"`          //: object, an embedded structure of the form Condition.
	ConditionName      string         `json:"condition_name"`     //: string, display name of the condition, same value as condition.displayName.
	Severity           string         `json:"severity"`           //: string, severity level of incidents. If this field isn't defined during alerting policy creation, then Cloud Monitoring sets the alerting policy severity to No Severity.
	ThresholdValue     string         `json:"threshold_value"`    //: string, the threshold value of this condition, may be empty if the condition isn't a threshold condition.
}

type GCPalert struct {
	Version  string           `json:"version"`
	Incident GCPAlertIncident `json:"incident"`
}
*/

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

func woop(w http.ResponseWriter, r *http.Request) {
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
	/*
		var alert GCPalert
		if err := json.NewDecoder(r.Body).Decode(&alert); err != nil {
			logger.Error("failed to decode body into struct.", err)
			return
		}
		logger.Debug(alert)
		logger.Info("Recieved an alert with summary: ", alert.Incident.Summary)
	*/

	logger.Info("Recieved an alert with summary: ", summary)

	fmt.Fprint(w, "connecting")
	ctx := context.Background()
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
