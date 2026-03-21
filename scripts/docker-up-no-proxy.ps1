# Сборка/запуск без системного HTTP(S)_PROXY (частая ошибка: 127.0.0.1:12334 refused).
# Запуск: powershell -ExecutionPolicy Bypass -File .\scripts\docker-up-no-proxy.ps1

$vars = @(
    'HTTP_PROXY', 'HTTPS_PROXY', 'http_proxy', 'https_proxy',
    'ALL_PROXY', 'all_proxy', 'NO_PROXY', 'no_proxy'
)
foreach ($v in $vars) {
    Remove-Item "Env:$v" -ErrorAction SilentlyContinue
}

Set-Location $PSScriptRoot\..
docker compose up -d --build @args
