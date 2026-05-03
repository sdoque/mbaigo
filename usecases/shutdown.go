/*******************************************************************************
 * Copyright (c) 2026 Synecdoque
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, subject to the following conditions:
 *
 * The software is licensed under the MIT License. See the LICENSE file in this repository for details.
 *
 * Contributors:
 *   Jan A. van Deventer, Luleå - initial implementation
 ***************************************************************************SDG*/

package usecases

import (
	"context"
	"log"

	"github.com/sdoque/mbaigo/components"
)

// WatchShutdown spawns a goroutine that waits for a shutdown signal on the
// system's Sigs channel and cancels the supplied context when one arrives.
//
// This must be called *early* in main — immediately after NewSystem and before
// any blocking call such as RequestCertificate's certificate-retry loop.
// Without it, a Ctrl+C delivered while main is blocked on the retry loop is
// silently absorbed by the Sigs channel: nothing reads from Sigs (because
// main has not yet reached `<-sys.Sigs`), nothing calls cancel(), and the
// retry loop's `<-ctx.Done()` check never fires. The system becomes
// un-killable except by SIGKILL or SIGQUIT.
//
// With this helper, the signal handler is wired up at startup and the
// context is cancelled the moment a signal arrives, regardless of what
// main is currently doing.
func WatchShutdown(sys *components.System, cancel context.CancelFunc) {
	go func() {
		sig := <-sys.Sigs
		log.Printf("shutdown signal %s received for %s", sig, sys.Name)
		cancel()
	}()
}
