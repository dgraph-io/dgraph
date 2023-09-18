import sys
import json
import torch
import torch.nn.functional as F
from transformers import AutoModel, AutoTokenizer, AutoConfig


def generate_embeddings(sentences, model_name):
    # Load model from HuggingFace Hub
    tokenizer = AutoTokenizer.from_pretrained(model_name)
    model = AutoModel.from_pretrained(model_name)

    # Tokenize sentences
    encoded_input = tokenizer(sentences, padding=True, truncation=True, return_tensors='pt')

    # Compute token embeddings
    with torch.no_grad():
        model_output = model(**encoded_input)

    # Perform pooling
    sentence_embeddings = mean_pooling(model_output, encoded_input['attention_mask'])
    # Normalize embeddings
    sentence_embeddings = F.normalize(sentence_embeddings, p=2, dim=1)
    return sentence_embeddings
#Mean Pooling - Take attention mask into account for correct averaging
def mean_pooling(model_output, attention_mask):
    token_embeddings = model_output[0] #First element of model_output contains all token embeddings
    input_mask_expanded = attention_mask.unsqueeze(-1).expand(token_embeddings.size()).float()
    return torch.sum(token_embeddings * input_mask_expanded, 1) / torch.clamp(input_mask_expanded.sum(1), min=1e-9)

def handler(event, context):
    body = json.loads(event['body'])
    print(body)
    text = body['text']
    t = generate_embeddings(text,"/var/task/model/")
    resp = []
  
    for idx, x in enumerate(text):
        resp.append(str(t[idx])[7:][:-1].replace('\n', ''))
  
    response = {"statusCode": 200, "body": json.dumps(resp)}

    return response
