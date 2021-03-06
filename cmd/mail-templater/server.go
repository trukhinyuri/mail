package main

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"context"
	"net/http"
	"os/signal"

	"git.containerum.net/ch/mail-templater/pkg/router"
	"git.containerum.net/ch/mail-templater/pkg/router/middleware"
	"github.com/containerum/kube-client/pkg/model"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

func initServer(c *cli.Context) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.TabIndent|tabwriter.Debug)
	for _, f := range c.GlobalFlagNames() {
		fmt.Fprintf(w, "Flag: %s\t Value: %q\n", f, c.String(f))
	}
	w.Flush()

	setupLogs(c)

	status := &model.ServiceStatus{
		Name:     c.App.Name,
		Version:  c.App.Version,
		StatusOK: true,
		Details:  map[string]string{},
	}

	ts, err := getTemplatesStorage(c)
	exitOnErr(err)
	defer ts.Close()
	ms, err := getMessagesStorage(c)
	exitOnErr(err)
	defer ms.Close()
	us, err := getUpstream(c, ms)
	exitOnErr(err)

	uss, userr := getUpstreamSimple(c, ms)
	if userr != nil {
		logrus.WithError(userr).Warn("simple upstream not ready")
		status.AddDetails("SimpleUpstreamNotReady", userr.Error())
		status.StatusOK = false
	}

	um, err := getUserManagerClient(c)
	exitOnErr(err)

	if err := importDefaultTemplates(c, ts); err != nil {
		logrus.WithError(err).Warn("unable to import default templates")
		status.AddDetails("UnableImportTemplates", err.Error())
	}

	app := router.CreateRouter(&middleware.Services{
		TemplateStorage:   ts,
		MessagesStorage:   ms,
		Upstream:          us,
		UpstreamSimple:    uss,
		UserManagerClient: um,
		Active:            userr == nil,
	}, status, c.Bool(corsFlag))

	// graceful shutdown support
	srv := http.Server{
		Addr:    ":" + c.String(portFlag),
		Handler: app,
	}

	go exitOnErr(srv.ListenAndServe())

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	logrus.Infoln("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}

func exitOnErr(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
