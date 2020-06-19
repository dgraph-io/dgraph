# **TESTING**


## **Installing Tests Tools**

### **Python VirtualEnv**

A recent version of python3 should be sufficient. I you have `pyenv` + `pyenv-virtualenv`, you setup a local virtual environment with this:

```bash
pyenv install 3.8.0
pyenv virtualenv 3.8.0 aws
cd .
```

### **Python Requirements**

```bash
pip install -r requirements.txt
```

## **Generate Key Pairs**

Generate keys and install them as keypairs into AWS regions.

```bash
./seed_keypairs.sh
```

## **Run Tests**

```bash
# copy template to current working directory
cp ../dgraph.json .
# run tests
taskcat test run
```
