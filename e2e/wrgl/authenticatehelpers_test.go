package e2e_wrgl_test

import (
	"bytes"
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/stretchr/testify/require"
)

type loginInstruction struct {
	VerificationURI string
	UserCode        string
}

func readLoginInstruction(t *testing.T, cmdOutput *bytes.Buffer) <-chan loginInstruction {
	t.Helper()
	ch := make(chan loginInstruction, 1)
	go func() {
		timeout := 10 * time.Second
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		defer close(ch)
		re := regexp.MustCompile(`.*Visit (.+) in your browser and enter user code "(.+)" to login.*`)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				m := re.FindStringSubmatch(cmdOutput.String())
				if m != nil {
					ch <- loginInstruction{m[1], m[2]}
					return
				}
			case <-ctx.Done():
				t.Errorf("readLoginInstruction timeout (%d)", timeout)
				return
			}
		}
	}()
	return ch
}

func userVerify(t *testing.T, instrChan <-chan loginInstruction) {
	t.Helper()
	instr := <-instrChan
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()
	require.NoError(t, chromedp.Run(ctx,
		chromedp.Navigate(instr.VerificationURI),
		chromedp.SendKeys(`#device-user-code`, instr.UserCode),
	))
	_, err := chromedp.RunResponse(ctx,
		chromedp.Submit("#kc-user-verify-device-user-code-form"),
	)
	require.NoError(t, err)
	_, err = chromedp.RunResponse(ctx,
		chromedp.SendKeys("#username", "johnd"),
		chromedp.SendKeys("#password", "password"),
		chromedp.Submit("#kc-form-login"),
	)
	require.NoError(t, err)
	_, err = chromedp.RunResponse(ctx,
		chromedp.Click("#kc-login"),
	)
	require.NoError(t, err)
}
