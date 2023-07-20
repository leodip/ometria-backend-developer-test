package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"

	"omimporter/internal/importer"
	"omimporter/internal/list"
	"omimporter/internal/statemgr"
	"strings"
	"time"

	"github.com/gookit/slog"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
)

func main() {

	// configure log template
	logTemplate := "[{{datetime}}] [{{level}}] {{message}}\n"
	f := slog.NewTextFormatter()
	f.SetTemplate(logTemplate)
	slog.SetFormatter(f)

	slog.Info("starting")

	var ctx = context.Background()
	initViper()
	testRedisConnection(ctx)

	stateManager := statemgr.NewRedis()
	importer, err := importer.NewImporter(
		viper.GetString("Mailchimp.BaseUrl"),
		viper.GetString("Mailchimp.ApiKey"),
		viper.GetString("Ometria.Endpoint"),
		viper.GetString("Ometria.ApiKey"),
		stateManager)
	if err != nil {
		panic(err)
	}

	// some arg flags
	var runIntervalInSeconds int
	flag.IntVar(&runIntervalInSeconds, "runIntervalInSeconds", viper.GetInt("RunIntervalInSeconds"), "The run interval in seconds")
	flag.Parse()

	slog.Info(fmt.Sprintf("will run every %v seconds", runIntervalInSeconds))

	// run every X seconds
	ticker := time.NewTicker(time.Duration(runIntervalInSeconds) * time.Second)

	doRun(ctx, importer)
	for range ticker.C {
		doRun(ctx, importer)
	}
}

func doRun(ctx context.Context, importer *importer.Importer) {
	slog.Info("starting run")

	lists, err := list.GetAllLists()
	if err != nil {
		slog.Error(err)
		return
	}

	slog.Info(fmt.Sprintf("found %v lists", len(lists)))
	bytes, err := json.MarshalIndent(lists, "", "    ")
	if err == nil {
		slog.Info(string(bytes))
	}

	nowTimestamp := time.Now()

	for _, l := range lists {
		slog.Info(fmt.Sprintf("sync'ing list %v (%v)", l.Id, l.Name))
		err := importer.SyncList(ctx, l.Id)
		if err != nil {
			slog.Error(err)
		}
	}

	elapsed := time.Since(nowTimestamp)

	slog.Info(fmt.Sprintf("finished run. Time elapsed: %v", elapsed))
}

func initViper() {
	viper.SetConfigName("config")
	viper.SetConfigType("json")

	// possible locations for config file
	viper.AddConfigPath(".")
	viper.AddConfigPath("./bin")
	viper.AddConfigPath("./configs")

	viper.SetEnvPrefix("OMIMPORTER")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	err := viper.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("unable to initialize configuration - make sure a config.json file exists and has content (%w)", err))
	}
	slog.Info(fmt.Sprintf("viper configuration initialized. Config file used: %v", viper.ConfigFileUsed()))
}

func testRedisConnection(ctx context.Context) {
	redisClient := redis.NewClient(&redis.Options{
		Addr:     viper.GetString("Redis.Addr"),
		Password: viper.GetString("Redis.Password"),
		DB:       viper.GetInt("DB"),
	})
	if err := redisClient.Ping(ctx); err.Err() != nil {
		panic(fmt.Errorf("unable to connect to redis - make sure redis is available on '%v' (%w)", viper.GetString("Redis.Addr"), err.Err()))
	}
}
