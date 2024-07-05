# Cloud function for the Woot Woot bot.
It is built with the viam micrordk on an esp32 w-rover with leds on pin 12 and a buzzer on pin 14.

## app.viam.com Config
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
          12,
          14
        ]
      }
    }
  ]
}
```

## Messages
Multiple types of messages are supported, depending on the hardware.

### RGB Control in Body Example
Note: The URL suffix and secret are stored as env variables

Assumes a buzzer on pin 5, Red on pin 19, Green on pin 18, and Blue on pin 21.

#### Message structure
```json
{ 
  "buzzer" : false,
  "red" : {"freq": 1000, "duty": 1},
  "green" : {"freq": 1000, "duty": 0.7},
  "blue" : {"freq": 1000, "duty": 0}
}
```
buzzer: true or false
red/green/blue: The PWM frequency and duty cycle is defined for each color, which a minimum frequency of 10. This allows for fine-grained control of each color.


#### Example
```bash
curl -m 130 -X POST 'https://us-central1-some-project.cloudfunctions.net/woopwoopV3-1?v=3&woop=4&secret=xyz'  -H "Content-Type: application/json" -d \
'{ 
  "buzzer" : false,
  "red" : {"freq": 1000, "duty": 1},
  "green" : {"freq": 1000, "duty": 0.7},
  "blue" : {"freq": 1000, "duty": 0}
}'
```

### Query Parameter Example
Note: The URL suffix and secret are stored as env variables

The fuction supports 4 parameters:
- secret - secret to authenticate this call
- woop - which woop to activate (see code for uri naming assumption)
- buzzer - control the buzzer on pin 14. Values: on|off
- strobe - control the led strobe on pin 12. Values: on|off

`curl -X GET 'https://us-central1-some-project.cloudfunctions.net/woop?woop=1\&secret=xyz&strobe=off&buzzer=on'`


### GCP Alert Example

#### Message structure
```json
{
    "incident": {
        "summary": "something for logging",
        "state" : "OPEN|CLOSED"
    }
}
```

#### Example Usage (without auth)
```bash
curl -m 130 -X POST https://gcp-project-123.cloudfunctions.net/woopwoop  -H "Content-Type: application/json" -d '{"incident": { "summary": "test", "state": "CLOSED"}}'
```
