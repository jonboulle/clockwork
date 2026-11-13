# clockwork

[![Mentioned in Awesome Go](https://awesome.re/mentioned-badge-flat.svg)](https://github.com/avelino/awesome-go#utilities)

[![GitHub Workflow Status](https://img.shields.io/github/actions/workflow/status/jonboulle/clockwork/ci.yaml?style=flat-square)](https://github.com/jonboulle/clockwork/actions?query=workflow%3ACI)
[![Go Report Card](https://goreportcard.com/badge/github.com/jonboulle/clockwork?style=flat-square)](https://goreportcard.com/report/github.com/jonboulle/clockwork)
![Go Version](https://img.shields.io/badge/go%20version-%3E=1.15-61CFDD.svg?style=flat-square)
[![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white&style=flat-square)](https://pkg.go.dev/mod/github.com/jonboulle/clockwork)

**A simple fake clock for Go.**


## Usage

Replace uses of the `time` package with the `clockwork.Clock` interface instead.

For example, instead of using `time.Sleep` directly:

```go
func myFunc() {
	time.Sleep(3 * time.Second)
	doSomething()
}
```

Inject a clock and use its `Sleep` method instead:

```go
func myFunc(clock clockwork.Clock) {
	clock.Sleep(3 * time.Second)
	doSomething()
}
```

Now you can easily test `myFunc` with a `FakeClock`:

```go
func TestMyFunc(t *testing.T) {
	ctx := context.Background()
	c := clockwork.NewFakeClock()

	// Start our sleepy function
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		myFunc(c)
		wg.Done()
	}()

	// Ensure we wait until myFunc is waiting on the clock.
	// Use a context to avoid blocking forever if something
	// goes wrong.
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	c.BlockUntilContext(ctx, 1)

	assertState()

	// Advance the FakeClock forward in time
	c.Advance(3 * time.Second)

	// Wait until the function completes
	wg.Wait()

	assertState()
}
```

and in production builds, simply inject the real clock instead:

```go
myFunc(clockwork.NewRealClock())
```

See [example_test.go](example_test.go) for a full example.

### Waiting for timers and tickers

A FakeClock "waiter" is a registered timer or ticker, including the timers used
by `Sleep` and `After`. Creating a timer with a positive duration or a ticker
registers it immediately. Calling `Chan()` or receiving from the channel does not
add a waiter. A timer's registration is removed when it fires or is stopped;
resetting it with a positive duration registers it again. Tickers register their
next expiration after each tick.

`BlockUntilContext(ctx, n)` waits for **at least** `n` registrations. For example,
the first call below returns immediately even though nothing has read the timer's
channel:

```go
func TestTimerWaiters(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	c := clockwork.NewFakeClock()
	timer := c.NewTimer(time.Hour)
	defer timer.Stop()

	if err := c.BlockUntilContext(ctx, 1); err != nil {
		t.Fatal(err)
	}

	timer.Stop()
	timer.Reset(2 * time.Hour)
	if err := c.BlockUntilContext(ctx, 1); err != nil {
		t.Fatal(err)
	}

	c.Advance(2 * time.Hour)
	select {
	case <-timer.Chan():
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
```

The count does not show whether a goroutine has received a timer event or
finished processing it. Use a channel or `sync.WaitGroup`, as in the earlier
example, to synchronize that work. Likewise, if timers are stopped and reset
concurrently, the same count can be present both before and after the reset;
waiting for that count does not establish that a particular reset has happened.


# Credits

clockwork is inspired by @wickman's [threaded fake clock](https://gist.github.com/wickman/3840816), and the [Golang playground](https://blog.golang.org/playground#TOC_3.1.)


## License

Apache License, Version 2.0. Please see [License File](LICENSE) for more information.
