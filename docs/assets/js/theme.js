/*
 * Theme selection: light, dark, or follow the system.
 *
 * The console's semantics (console/ui/src/theme/useTheme.ts) without Zustand.
 * SYSTEM IS THE DEFAULT, and it is a live subscription rather than a read at
 * startup — a reader whose OS flips to dark in the evening should not have to
 * reload a page they left open. That is also why storage holds the CHOICE
 * ("system") and the applied theme is derived: a resolved value would forget
 * that the reader asked to follow.
 *
 * Its own key. `agentops-console-theme` belongs to the console; a docs site
 * writing it would be reaching into another app's namespace — and on a shared
 * origin, literally so.
 *
 * The apply step also ships as a blocking inline script in <head>, which is
 * what stops a stored dark choice flashing light on every navigation. This file
 * wires the control and the subscription; it does not own first paint.
 */
(function () {
  'use strict'

  var KEY = 'agentops-docs-theme'
  var QUERY = '(prefers-color-scheme: dark)'
  var CLASS = { light: 'pf-v6-theme-light', dark: 'pf-v6-theme-dark' }

  function systemTheme() {
    return window.matchMedia && window.matchMedia(QUERY).matches ? 'dark' : 'light'
  }

  function resolve(choice) {
    return choice === 'system' ? systemTheme() : choice
  }

  function readChoice() {
    try {
      var stored = localStorage.getItem(KEY)
      return stored === 'light' || stored === 'dark' || stored === 'system' ? stored : 'system'
    } catch (e) {
      return 'system'
    }
  }

  function writeChoice(choice) {
    try {
      localStorage.setItem(KEY, choice)
    } catch (e) {
      /* private mode: the choice applies for this page, it just does not persist */
    }
  }

  /** apply puts the resolved theme on <html>, where the token blocks read it. */
  function apply(theme) {
    var root = document.documentElement
    root.classList.remove(CLASS.light, CLASS.dark)
    root.classList.add(CLASS[theme])
    // The attribute is the hook for plain CSS that should not have to know
    // PatternFly's class names; colorScheme tells the browser which scrollbar
    // and form-control colours to use, so the chrome matches the page.
    root.setAttribute('data-theme', theme)
    root.style.colorScheme = theme
  }

  var buttons = document.querySelectorAll('[data-theme-choice]')

  function reflect(choice) {
    for (var i = 0; i < buttons.length; i++) {
      var btn = buttons[i]
      btn.setAttribute(
        'aria-pressed',
        btn.getAttribute('data-theme-choice') === choice ? 'true' : 'false',
      )
    }
  }

  function select(choice) {
    writeChoice(choice)
    apply(resolve(choice))
    reflect(choice)
  }

  for (var i = 0; i < buttons.length; i++) {
    ;(function (btn) {
      btn.addEventListener('click', function () {
        select(btn.getAttribute('data-theme-choice'))
      })
    })(buttons[i])
  }

  var choice = readChoice()
  apply(resolve(choice))
  reflect(choice)

  if (window.matchMedia) {
    var mq = window.matchMedia(QUERY)
    var onSystemChange = function () {
      if (readChoice() === 'system') apply(systemTheme())
    }
    if (mq.addEventListener) mq.addEventListener('change', onSystemChange)
    else if (mq.addListener) mq.addListener(onSystemChange)
  }
})()
