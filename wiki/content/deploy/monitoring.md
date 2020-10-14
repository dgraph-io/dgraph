+++
date = "2017-03-20T22:25:17+11:00"
title = "Monitoring"
weight = 14
[menu.main]
    parent = "deploy"
+++

Dgraph exposes metrics via the `/debug/vars` endpoint in json format and the `/debug/prometheus_metrics` endpoint in Prometheus's text-based format. Dgraph doesn't store the metrics and only exposes the value of the metrics at that instant. You can either poll this endpoint to get the data in your monitoring systems or install **[Prometheus](https://prometheus.io/docs/introduction/install/)**. Replace targets in the below config file with the ip of your Dgraph instances and run prometheus using the command `prometheus --config.file my_config.yaml`.

```sh
scrape_configs:
  - job_name: "dgraph"
    metrics_path: "/debug/prometheus_metrics"
    scrape_interval: "2s"
    static_configs:
    - targets:
      - 172.31.9.133:6080     # For Dgraph zero, 6080 is the http endpoint exposing metrics.
      - 172.31.15.230:8080    # For Dgraph alpha, 8080 is the http endpoint exposing metrics.
      - 172.31.0.170:8080
      - 172.31.8.118:8080
```

{{% notice "note" %}}
Raw data exported by Prometheus is available via `/debug/prometheus_metrics` endpoint on Dgraph alphas.
{{% /notice %}}

Install **[Grafana](http://docs.grafana.org/installation/)** to plot the metrics. Grafana runs at port 3000 in default settings. Create a prometheus datasource by following these **[steps](https://prometheus.io/docs/visualization/grafana/#creating-a-prometheus-data-source)**. Import **[grafana_dashboard.json](https://github.com/dgraph-io/benchmarks/blob/master/scripts/grafana_dashboard.json)** by following this **[link](http://docs.grafana.org/reference/export_import/#importing-a-dashboard)**.


## CloudWatch

Route53's health checks can be leveraged to create standard CloudWatch alarms to notify on change in the status of the `/health` endpoints of Alpha and Zero.

Considering that the endpoints to monitor are publicly accessible and you have the AWS credentials and [awscli](https://aws.amazon.com/cli/) setup, we’ll go through an example of setting up a simple CloudWatch alarm configured to alert via email for the Alpha endpoint `alpha.acme.org:8080/health`. Dgraph Zero's `/health` endpoint can also be monitored in a similar way.



### Create the Route53 Health Check
```sh
aws route53 create-health-check \
    --caller-reference $(date "+%Y%m%d%H%M%S") \
    --health-check-config file:///tmp/create-healthcheck.json \
    --query 'HealthCheck.Id'
```
The file `/tmp/create-healthcheck.json` would need to have the values for the parameters required to create the health check as such:
```sh
{
  "Type": "HTTPS",
  "ResourcePath": "/health",
  "FullyQualifiedDomainName": "alpha.acme.org",
  "Port": 8080,
  "RequestInterval": 30,
  "FailureThreshold": 3
}
```
The reference for the values one can specify while creating or updating a health check can be found on the AWS [documentation](https://docs.aws.amazon.com/Route53/latest/DeveloperGuide/health-checks-creating-values.html).

The response to the above command would be the ID of the created health check.
```sh
"29bdeaaa-f5b5-417e-a5ce-7dba1k5f131b"
```
Make a note of the health check ID. This will be used to integrate CloudWatch alarms with the health check.

{{% notice "note" %}}
Currently, Route53 metrics are only (available)[https://docs.aws.amazon.com/Route53/latest/DeveloperGuide/monitoring-health-checks.html] in the **US East (N. Virginia)** region. The Cloudwatch Alarm (and the SNS Topic) should therefore be created in `us-east-1`.
{{% /notice %}}

### [Optional] Creating an SNS Topic
SNS topics are used to create message delivery channels. If you do not have any SNS topics configured, one can be created by running the following command:

```sh
aws sns create-topic --region=us-east-1 --name ops --query 'TopicArn'
```

The response to the above command would be as follows:
```sh
"arn:aws:sns:us-east-1:123456789012:ops"
```
Be sure to make a note of the topic ARN. This would be used to configure the CloudWatch alarm's action parameter.

Run the following command to subscribe your email to the SNS topic:
```sh
aws sns subscribe \
    --topic-arn arn:aws:sns:us-east-1:123456789012:ops \
    --protocol email \
    --notification-endpoint ops@acme.org
```
The subscription will need to be confirmed through *AWS Notification - Subscription Confirmation* sent through email. Once the subscription is confirmed, CloudWatch can be configured to use the SNS topic to trigger the alarm notification.



### Creating a CloudWatch Alarm
The following command creates a CloudWatch alarm with `--alarm-actions` set to the ARN of the SNS topic and the `--dimensions` of the alarm set to the health check ID.
```sh
aws cloudwatch put-metric-alarm \
    --region=us-east-1 \
    --alarm-name dgraph-alpha \
    --alarm-description "Alarm for when Alpha is down" \
    --metric-name HealthCheckStatus \
    --dimensions "Name=HealthCheckId,Value=29bdeaaa-f5b5-417e-a5ce-7dba1k5f131b" \
    --namespace AWS/Route53 \
    --statistic Minimum \
    --period 60 \
    --threshold 1 \
    --comparison-operator LessThanThreshold \
    --evaluation-periods 1 \
    --treat-missing-data breaching \
    --alarm-actions arn:aws:sns:us-east-1:123456789012:ops
```

One can verify the alarm status from the CloudWatch or Route53 consoles.

#### Internal Endpoints
If the Alpha endpoint is internal to the VPC network - one would need to create a Lambda function that would periodically (triggered using CloudWatch Event Rules) request the `/health` path and create CloudWatch metrics which could then be used to create the required CloudWatch alarms.
The architecture and the CloudFormation template to achieve the same can be found [here](https://aws.amazon.com/blogs/networking-and-content-delivery/performing-route-53-health-checks-on-private-resources-in-a-vpc-with-aws-lambda-and-amazon-cloudwatch/).
