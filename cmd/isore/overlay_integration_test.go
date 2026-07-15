package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/joaomdsg/isore/internal/browser"
	"github.com/stretchr/testify/require"
)

// overlayPage drives the real overlay JS in a headless browser.
type overlayPage struct {
	t *testing.T
	d *browser.Driver
}

func newOverlayPage(t *testing.T) *overlayPage {
	t.Helper()
	chrome := browser.FindChrome()
	if chrome == "" {
		t.Skip("no chromium/chrome binary")
	}
	d, err := browser.Launch(context.Background(), chrome)
	require.NoError(t, err)
	t.Cleanup(d.Close)
	require.NoError(t, d.Navigate("about:blank"))
	_, err = d.Eval(overlayJS)
	require.NoError(t, err)
	return &overlayPage{t: t, d: d}
}

func (p *overlayPage) eval(expr string) json.RawMessage {
	p.t.Helper()
	out, err := p.d.Eval(expr)
	require.NoError(p.t, err, "eval %s", expr)
	return out
}

func (p *overlayPage) evalBool(expr string) bool {
	var v bool
	require.NoError(p.t, json.Unmarshal(p.eval(expr), &v), "expr %s", expr)
	return v
}

// shadow query helper injected once.
const helpers = `window.__q=function(sel){return document.querySelector('[data-es-root]').shadowRoot.querySelector(sel);};true`

func (p *overlayPage) enable() {
	p.eval(helpers)
	p.eval(`__q('.es-switch input').click();true`)
	require.True(p.t, p.evalBool(`window.__es.enabled()`), "overlay must be enabled")
}

func (p *overlayPage) addNote(js string) {
	p.eval(`window.__es.notes().push(` + js + `);window.__es.ensure();true`)
}

func (p *overlayPage) clickBody() {
	p.eval(`document.body.click();true`)
}

func (p *overlayPage) inlineCardOpen() bool {
	return p.evalBool(`!!__q('[data-es=inline]')`)
}

func (p *overlayPage) closeInlineCard() {
	p.eval(`__q('[data-es=inline] [data-es=cancel]').click();true`)
}

func TestOverlayFreezesWhileAgentWorking(t *testing.T) {
	p := newOverlayPage(t)
	p.enable()

	// Positive control: annotating works when idle.
	p.clickBody()
	require.True(t, p.inlineCardOpen(), "click must open the note card when idle")
	p.closeInlineCard()

	// A note the agent picked up freezes the whole overlay.
	p.addNote(`{id:'n1',selector:'body',note:'x',url:location.href,createdAt:1,editedAt:1,dispatchedAt:1,agentStatus:'working'}`)
	p.clickBody()
	require.False(t, p.inlineCardOpen(), "click must not annotate while agent works")
	require.True(t, p.evalBool(`__q('.es-switch input').disabled`), "toggle must be frozen")
	require.True(t, p.evalBool(`__q('[data-es=dispatch]').disabled`), "dispatch must be frozen")

	// mark_fixed thaws it.
	p.eval(`(function(){var n=window.__es.notes()[0];n.agentStatus='fixed';n.fixedAt=2;window.__es.ensure();})();true`)
	require.False(t, p.evalBool(`__q('.es-switch input').disabled`), "toggle must thaw once fixed")
	p.clickBody()
	require.True(t, p.inlineCardOpen(), "annotating must resume once fixed")
	p.closeInlineCard()
}

func TestOverlayDispatchDisabledWithNothingToDispatch(t *testing.T) {
	p := newOverlayPage(t)
	p.enable()

	// A note already dispatched, fixed, and unedited since: nothing to dispatch.
	// (fixedAt matters: an outstanding, not-yet-fixed dispatch freezes the whole
	// overlay under the new 'dispatched' state, which is covered separately.)
	p.addNote(`{id:'n1',selector:'body',note:'x',url:location.href,createdAt:1,editedAt:1,dispatchedAt:5,fixedAt:6}`)
	require.True(t, p.evalBool(`__q('[data-es=dispatch]').disabled`), "dispatch must disable with 0 undispatched items")

	// A fresh note enables it.
	p.addNote(`{id:'n2',selector:'body',note:'y',url:location.href,createdAt:6,editedAt:6,dispatchedAt:0}`)
	require.False(t, p.evalBool(`__q('[data-es=dispatch]').disabled`), "dispatch must enable with a pending item")
}

func TestOverlayFreezesOnceDispatchedEvenBeforeTheAgentPicksItUp(t *testing.T) {
	p := newOverlayPage(t)
	p.enable()

	// Dispatched but no agentStatus yet: the agent hasn't called mark_working.
	p.addNote(`{id:'n1',selector:'body',note:'x',url:location.href,createdAt:1,editedAt:1,dispatchedAt:1}`)
	p.clickBody()
	require.False(t, p.inlineCardOpen(), "click must not annotate once dispatched, even pre-pickup")
	require.True(t, p.evalBool(`__q('.es-switch input').disabled`), "toggle must freeze on dispatch, not just on pickup")
	require.True(t, p.evalBool(`document.querySelector('[data-es=badge]').getAttribute('data-es-state')==='dispatched'`),
		"badge must show the distinct 'dispatched' color, not fall back to 'pending'")

	// mark_fixed thaws it, same as the working case.
	p.eval(`(function(){var n=window.__es.notes()[0];n.fixedAt=2;window.__es.ensure();})();true`)
	require.False(t, p.evalBool(`__q('.es-switch input').disabled`), "toggle must thaw once fixed")
}

func TestClickingDispatchFreezesTheOverlayImmediatelyNotAfterTheCelebrationTimeout(t *testing.T) {
	p := newOverlayPage(t)
	p.enable()
	p.addNote(`{id:'n1',selector:'body',note:'x',url:location.href,createdAt:1,editedAt:1,dispatchedAt:0}`)

	p.eval(`__q('[data-es=dispatch]').click();true`)

	require.True(t, p.evalBool(`__q('.es-switch input').disabled`),
		"toggle must freeze the instant Dispatch is clicked, not 1.4s later when the button's own celebration timeout fires")
	p.clickBody()
	require.False(t, p.inlineCardOpen(), "annotation must be blocked immediately on dispatch")
}

func TestCloseButtonActuallyDeletesTheNote(t *testing.T) {
	p := newOverlayPage(t)
	p.enable()
	p.addNote(`{id:'n1',selector:'body',note:'x',url:location.href,createdAt:1,editedAt:1,dispatchedAt:0}`)

	p.eval(`document.querySelector('[data-es=badge]').dispatchEvent(new Event('mouseenter'));true`)
	require.True(t, p.evalBool(`!!__q('[data-es=popover]')`), "hover must open the popover")

	p.eval(`__q('[data-es=pop-del]').click();true`)
	p.eval(`__q('[data-es=del-confirm]').click();true`)

	require.True(t, p.evalBool(`window.__es.notes().length===0`), "confirming Close must actually remove the note")
}
