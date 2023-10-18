<!--
title: 'AWS Simple HTTP Endpoint example in Python'
description: 'This template demonstrates how to make a simple HTTP API with Python running on AWS Lambda and API Gateway using the Serverless'
-->

# sentence transformer exposed as a serverless AWS Lambda function

This repo provides the artefacts and steps to expose an AWS Lambda function providing vector embedding for sentences based on huggingface model.


## Design
We deploy a Lambda function using a custom docker image build from public.ecr.aws/lambda/python:3.8.

The docker image contains python library, sentence transformer pre-trained model and the lambda function handler.

**Service signature**
The service accepts a map of IDs and sentences
```json
{
    "0x01":"some text",
    "0x02":"other sentence
}
```
and returns a map of IDs and vectors.
```json
{
    "0x01":"[-0.010852468200027943, -0.016728922724723816, ...]",
    ...
}
```

We opted for a map vs an array of vectors to support parallelism if needed.
By using IDs, we don't have to provide the vectors in the same order as the sentences.

Client should use the IDs to associate the vectors with the right objects.


## Usage
Some steps are still manual in this version but can be automated.


Following https://www.philschmid.de/serverless-bert-with-huggingface-aws-lambda-docker
### Set your AWS CLI 
have aws cli installed and a profile [dgraph] in ~/.aws/credential with aws_access_key_id and aws_secret_access_key.

### Create ECR repository
aws_region=us-east-1
aws_account_id=<account_id>

> aws ecr create-repository --repository-name embedding-lambda --region $aws_region --profile dgraph
### build and tag the docker image
> docker build -t python-lambda .
use ECR repositoryUri to tag the image
> docker tag python-lambda $aws_account_id.dkr.ecr.us-east-1.amazonaws.com/embedding-lambda

### push the image to ECR
> aws ecr get-login-password --region $aws_region --profile dgraph\
| docker login \
    --username AWS \
    --password-stdin $aws_account_id.dkr.ecr.us-east-1.amazonaws.com/embedding-lambda
Login Succeeded

> docker push $aws_account_id.dkr.ecr.us-east-1.amazonaws.com/embedding-lambda

### deploy the serverless function
set the image reference in serverless.yml config.

Deploy to AWS
```
$ serverless deploy --aws-profile dgraph
```

If we build an arm64 docker image (on Mac without forcing the platform), then the lambda must be configured with
- architecture: arm64
else, if we build the docker image for platform linux/amd64, then set
- architecture: x86_64

_Note_: In current form, after deployment, your API is protected by an API key. 

For production deployments, you might want to configure an authorizer.

### Invocation
```
curl --request POST \

--url https://<>.execute-api.us-east-1.amazonaws.com/dev/embedding \
--header 'Content-Type: application/json' --header 'x-api-key: <apikey>' \
--data '{"id":"some sample text"}'
```


### Local development

> docker build -t embedding-lambda .

Build for a specific platform

> docker build -t embedding-lambda .

docker run -d --name embedding -p 8180:8080  -v ./:/var/task/   python-lambda

Runing locally a specific handler:

> docker run -d --name embedding -p 8180:8080  -v ./handler.py:/var/task/handler.py   python-lambda "handler.embedding"

```
curl --request POST \
--url http://localhost:8180/2015-03-31/functions/function/invocations \
--header 'Content-Type: application/json' \
--data '{"body":"{\"id1\":\"some sample text\",\"id2\":\"some other text\"\n}"}'
```

Note that the payload has a “body” element as a string.

