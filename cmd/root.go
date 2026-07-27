package cmd

import (
	"net/http"
	"net/http/pprof"
	"runtime"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/lib/config"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/spf13/viper"
)

var cfgFile string
var debugLogging bool
var prettyLogs bool
var pprofAddr string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "sukyan",
	Short: `A web application vulnerability scanner`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		lib.ZeroConsoleAndFileLog()
		if debugLogging {
			zerolog.SetGlobalLevel(zerolog.DebugLevel)
		} else {
			zerolog.SetGlobalLevel(zerolog.InfoLevel)
		}
		startPprofServer()
		return nil
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		db.Cleanup()
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.sukyan.yaml)")
	rootCmd.PersistentFlags().BoolVar(&debugLogging, "debug", false, "Use debug level logging")
	rootCmd.PersistentFlags().BoolVar(&prettyLogs, "pretty", true, "Use pretty logging instead JSON")
	rootCmd.PersistentFlags().StringVar(&pprofAddr, "pprof-addr", "", "Expose net/http/pprof profiling endpoints on this address (e.g. 127.0.0.1:6060); disabled when empty")

}

// startPprofServer exposes the runtime profiling endpoints on an isolated mux so
// the debug surface is never attached to DefaultServeMux or any product listener.
func startPprofServer() {
	if pprofAddr == "" {
		return
	}
	// Sampled rather than exhaustive: enough to rank contention without paying
	// the full accounting cost on every block/unlock.
	runtime.SetBlockProfileRate(10_000_000)
	runtime.SetMutexProfileFraction(100)

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	server := &http.Server{
		Addr:              pprofAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Info().Str("addr", pprofAddr).Msg("pprof profiling server listening")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Str("addr", pprofAddr).Msg("pprof profiling server stopped")
		}
	}()
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	}
	config.LoadConfig()
	viper.AutomaticEnv()
}
