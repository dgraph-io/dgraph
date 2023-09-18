import json
import os
from sentence_transformers import SentenceTransformer

# change CACHE else we get following error at runtime in AWS Lambda:
# There was a problem when trying to write in your cache folder (/home/sbx_user1051/.cache/huggingface/hub).
os.environ['TRANSFORMERS_CACHE'] = '/var/task'

def handler(event, context):
    body = json.loads(event['body'])
    sentences = body['text']
    model = SentenceTransformer('/var/task/model')
    embeddings = model.encode(sentences)

    resp = []
  
    for v in embeddings:
        resp.append(json.dumps(v.tolist()))
        
    response = {"statusCode": 200, "body": json.dumps(resp)}

    return response
