package setup

import htmltemplate "html/template"

const setupLandingHTML = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>LiveReview Setup</title>
  <style>
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      display: flex;
      justify-content: center;
      align-items: center;
      min-height: 100vh;
      margin: 0;
      background: #f5f5f5;
      color: #333;
    }
    .card {
      background: white;
      border-radius: 12px;
      padding: 48px;
      box-shadow: 0 2px 12px rgba(0,0,0,0.1);
      text-align: center;
      max-width: 480px;
    }
    h1 { margin: 0 0 16px; font-size: 24px; }
    p { color: #666; line-height: 1.5; }
    a { color: #4F46E5; }
    .spinner {
      width: 40px; height: 40px;
      border: 4px solid #e5e7eb;
      border-top-color: #4F46E5;
			border-radius: 50%;
      animation: spin 0.8s linear infinite;
      margin: 0 auto 24px;
    }
    @keyframes spin { to { transform: rotate(360deg); } }
  </style>
</head>
<body>
  <div class="card">
    <div class="spinner"></div>
    <h1>Redirecting to Hexmos Login</h1>
		<p>You'll be redirected automatically. If not, <a href="{{.SigninURL}}">click here</a>.</p>
  </div>
	<script>window.location.href = "{{.SigninURL}}";</script>
</body>
</html>`

const setupSuccessHTML = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>LiveReview Setup - Success</title>
  <style>
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      display: flex;
      justify-content: center;
      align-items: center;
      min-height: 100vh;
      margin: 0;
      background: #f5f5f5;
      color: #333;
    }
    .card {
      background: white;
      border-radius: 12px;
      padding: 48px;
      box-shadow: 0 2px 12px rgba(0,0,0,0.1);
      text-align: center;
      max-width: 480px;
    }
    h1 { margin: 0 0 16px; font-size: 24px; color: #059669; }
    p { color: #666; line-height: 1.5; }
    .check {
      width: 48px; height: 48px;
      background: #059669;
      border-radius: 50%;
      display: flex;
      align-items: center;
      justify-content: center;
      margin: 0 auto 24px;
      color: white;
      font-size: 24px;
    }
  </style>
</head>
<body>
  <div class="card">
    <div class="check">&#10003;</div>
    <h1>Authentication Successful</h1>
    <p>You can close this tab and return to your terminal to complete the setup.</p>
  </div>
</body>
</html>`

const setupErrorHTML = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>LiveReview Setup - Error</title>
  <style>
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      display: flex;
      justify-content: center;
      align-items: center;
      min-height: 100vh;
      margin: 0;
      background: #f5f5f5;
      color: #333;
    }
    .card {
      background: white;
      border-radius: 12px;
      padding: 48px;
      box-shadow: 0 2px 12px rgba(0,0,0,0.1);
      text-align: center;
      max-width: 480px;
    }
    h1 { margin: 0 0 16px; font-size: 24px; color: #DC2626; }
    p { color: #666; line-height: 1.5; }
    .icon {
      width: 48px; height: 48px;
      background: #DC2626;
      border-radius: 50%;
      display: flex;
      align-items: center;
      justify-content: center;
      margin: 0 auto 24px;
      color: white;
      font-size: 24px;
    }
  </style>
</head>
<body>
  <div class="card">
    <div class="icon">&#10007;</div>
    <h1>Authentication Failed</h1>
    <p>Something went wrong. Please close this tab and try running <code>lrc setup</code> again.</p>
  </div>
</body>
</html>`

var SetupLandingPageTemplate = htmltemplate.Must(htmltemplate.New("setup-landing").Parse(setupLandingHTML))
var SetupSuccessPageTemplate = htmltemplate.Must(htmltemplate.New("setup-success").Parse(setupSuccessHTML))
var SetupErrorPageTemplate = htmltemplate.Must(htmltemplate.New("setup-error").Parse(setupErrorHTML))
