+++
date = "2017-03-20T22:25:17+11:00"
title = "Using the Dgraph Sentry Integration"
weight = 3
[menu.main]
    parent = "howto"
+++

Sentry is a powerful service that allows applications to send arbitrary events, messages, exceptions, bread-crumbs (logs) to your sentry account. In simplest terms, it is a dial-home service but also has a  rich feature set including event filtering, data scrubbing, several SDKs, custom and release tagging, as well as integration with 3rd party tools such as Slack, GitHub.

Although Sentry reporting is on by default, starting from v20.03.1 and v20.07.0, there is a configuration flag `enable-sentry` which can be used to completely turn off Sentry events reporting. 

## Basic Integration

**Panics (runtime and manual)**

* As of now, at Dgraph, we use Sentry reporting for capturing panics only. For manual panics anywhere in the code, sentry.CaptureException() API is called. 

* For runtime panics, Sentry does not have any native method. After further research, we chose the approach of a wrapper process to capture these panics. The basic idea for this is that whenever a dgraph instance is started, a 2nd monitoring process is started whose only job is to monitor the stderr for panics of the monitored process. When a panic is seen, it is reported back to sentry via the CaptureException API. 

**Reporting**

Each event is tagged with the release version, environment, timestamp, tags and the panic backtrace as explained below.
**Release:**

  - This is the release version string of the Dgraph instance.
  
**Environments:**

We have defined 4 environments:

**dev-oss / dev-enterprise**: These are events seen on non-released / local developer builds.

**prod-oss/prod-enterprise**: These are events on released version such as v20.03.0. Events in this category are also sent on a slack channel private to Dgraph

**Tags:**

Tags are key-value pairs that provide additional context for an event. We have defined the following tags:

`dgraph`: This tag can have values “zero” or “alpha” depending on which sub-command saw the panic/exception.

## Data Handling

We strive to handle your data with care in a variety of ways when sending events to Sentry

1. **Event Selection:** As of now, only panic events are sent to Sentry from Dgraph. 
2. **Data in Transit:** Events sent from the SDK to the Sentry server are encrypted on the wire with industry-standard TLS protocol with 256 bit AES Cipher.
3. **Data at rest:** Events on the Sentry server are also encrypted with 256 bit AES cipher. Sentry is hosted on GCP and as such physical access is tightly controlled. Logical access is only available to sentry approved officials.
4. **Data Retention:** Sentry stores events only for 90 days after which they are removed permanently.
5. **Data Scrubbing**: The Data Scrcubber option (default: on) in Sentry’s settings ensures PII doesn’t get sent to or stored on Sentry’s servers, automatically removing any values that look like they contain sensitive information for values that contain various strings. The strings we currently monitor and scrub are:

- `password`
- `secret`
- `passwd`
- `api_key`
- `apikey`
- `access_token`
- `auth_token`
- `credentials`
- `mysql_pwd`
- `stripetoken`
- `card[number]`
- `ip addresses`