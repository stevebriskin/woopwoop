Cloud function for the Woot Woot bot.

app.viam.com Config
```json
{
  "components": [
    {
      "name": "board",
      "namespace": "rdk",
      "type": "board",
      "model": "esp32",
      "attributes": {
        "pins": [
          12
        ]
      }
    }
  ]
}
```

Message structure
```json
{
    "incident": {
        "summary": "something for logging",
        "state" : "OPEN|CLOSED"
    }
}
```

Example Usage (without auth)
```bash
curl -m 130 -X POST https://gcp-project-123.cloudfunctions.net/woopwoop  -H "Content-Type: application/json" -d '{"incident": { "summary": "test", "state": "CLOSED"}}'
```
