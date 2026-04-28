export const config = {
  // Empty string means relative to current domain (centralized through Caddy)
  apiUrl: '',
  // Construct WS URL from current location (centralized through Caddy)
  wsUrl: `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}`,
}
