curl -fsSL https://hexmos.com/lrc-install.sh | LRC_RELEASE_CHANNEL=internal bash

$env:LRC_RELEASE_CHANNEL="internal"
iwr -useb https://hexmos.com/lrc-install.ps1 | iex