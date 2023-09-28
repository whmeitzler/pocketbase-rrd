# Pocketbase Round Robin Database

*The heck is a Round Robin Database?*

A fixed size buffer. We have plenty of names for them:
[Circular Buffers](https://hackaday.com/2015/10/29/embed-with-elliot-going-round-with-circular-buffers/)

[Round Robin Database](https://oss.oetiker.ch/rrdtool/)

[Capped Collection](https://www.mongodb.com/docs/manual/core/capped-collections/)


Usage:
```go
app := pocketbase.New()

rrd.MustRegister(app, app.RootCmd, rrd.Config{
		ConfigCollection: "signals",
		RingName:         "collection",
		SizeColumnName:   "size",
	})

```
Expects (**but does not create**) a collection by the name of `signals` that has a numeric column `size` and a plaintext column `collection`. This can be configured as seen above. 

Collections that are referred to in the above table are then "hooked into" and begin to behave like a circular buffer. If there are more records in the table than the limit, the table is truncated. 
Such collections can only be added to- no updates or deletions. Once the collection hits the prescribed limit, inserts are translated into updates. Deletes and modifications are disallowed.

This is **experimental software** and the only guarantee is that it's buggy and unsupported. 
