var React = require('react');
var ReactRouter = require('react-router');
var Link = ReactRouter.Link;
require('whatwg-fetch');
var vis = require('vis');

require('brace');
var AceEditor = require('react-ace').default;
require('brace/mode/logiql');
require('brace/theme/github');

var NavBar = require('./Navbar');

var styles = require('../styles');
var graph = styles.graph;
var stats = styles.statistics;
var editor = styles.editor;

// TODO - Abstract these out and remove these from global scope.
var predLabel = {};
var edgeLabels = {};
var uidMap = {};
var nodes = [],
  edges = [],
  network;

// TODO - Move these three functions to server or to some other component. They dont
// really belong here.
function getLabel(properties) {
  var label = "";
  if (properties["name"] != undefined) {
    label = properties["name"]
  } else if (properties["name.en"] != undefined) {
    label = properties["name.en"]
  } else {
    label = properties["_uid_"]
  }

  var words = label.split(" ")
  if (words.length > 1) {
    label = words[0] + "\n" + words[1]
    if (words.length > 2) {
      label += "..."
    }
  }
  return label
}

function getPredProperties(pred) {
  var prop = predLabel[pred]
  if (prop != undefined) {
    // We have already calculated the label for this predicate.
    return prop
  }

  var l
  for (var i = 1; i <= pred.length; i++) {
    l = pred.substr(0, i)
    if (edgeLabels[l] == undefined) {
      // This label hasn't been allocated yet.
      predLabel[pred] = {
        'label': l,
        // 'color': randomColor()
      };
      edgeLabels[l] = true;
      break;
    }
    // If it has already been allocated, then we increase the substring length and look again.
  }
  if (l == undefined) {
    predLabel[pred] = {
      'label': pred,
      // 'color': randomColor()
    };
    edgeLabels[l] = true;
  }
  return predLabel[pred]
}

function processObject(obj, src) {
  var properties = {}
  for (var prop in obj) {
    // If its just a value, then we store it in properties for this node.
    if (!Array.isArray(obj[prop])) {
      properties[prop] = obj[prop]
    } else {
      var arr = obj[prop]
      for (var i = 0; i < arr.length; i++) {
        processObject(arr[i], {
          id: obj["_uid_"],
          pred: prop
        })
      }
    }

  }

  var predProperties = getPredProperties(src.pred)
  if (!uidMap[obj["_uid_"]]) {
    uidMap[obj["_uid_"]] = true

    nodes.push({
      id: obj["_uid_"],
      label: getLabel(properties),
      title: JSON.stringify(properties),
      group: src.pred
    })
  }

  if (src.pred != "") {
    edges.push({
      from: src.id,
      to: obj["_uid_"],
      title: src.pred,
      label: predProperties["label"],
      arrows: 'to'
    })
  }
}

// create a network
function renderNetwork(nodes, edges) {
  var container = document.getElementById('graph');
  var data = {
    nodes: new vis.DataSet(nodes),
    edges: new vis.DataSet(edges)
  };
  var options = {
    nodes: {
      shape: 'circle',
      font: {
        size: 16
      },
      margin: {
        top: 25
      }
    },
    height: '100%',
    width: '100%',
    interaction: {
      hover: true,
      tooltipDelay: 1000
    },
  };

  network = new vis.Network(container, data, options);
}


function checkStatus(response) {
  if (response.status >= 200 && response.status < 300) {
    return response
  } else {
    var error = new Error(response.statusText)
    error.response = response
    throw error
  }
}

function parseJSON(response) {
  return response.json()
}

function timeout(ms, promise) {
  return new Promise(function(resolve, reject) {
    setTimeout(function() {
      reject(new Error("timeout"))
    }, ms)
    promise.then(resolve, reject)
  })
}

var Home = React.createClass({
  getInitialState: function() {
    return {
      query: '',
      response: '',
      latency: '',
      rendering: '',
      resType: ''
    }
  },
  queryChange: function(newValue) {
    // TODO - Maybe store this at a better place once you figure it out.
    this.query = newValue;
  },
  // TODO - Dont send mutation. query if text didn't change.
  runQuery: function(e) {
    e.preventDefault();

    // Resetting state
    predLabel = {}, edgeLabels = {}, uidMap = {}, nodes = [], edges = [];
    network && network.destroy();
    this.setState({
      rendering: '',
      latency: '',
      resType: 'hourglass',
      response: ''
    })

    var that = this;
    timeout(60000, fetch('http://localhost:8080/query?debug=true', {
        method: 'POST',
        mode: 'cors',
        headers: {
          'Content-Type': 'text/plain',
        },
        body: this.query
      }).then(checkStatus)
      .then(parseJSON)
      .then(function(result) {
        that.setState({
          response: '',
          resType: ''
        })
        var key = Object.keys(result)[0];
        if (result.code != undefined && result.message != undefined) {
          // This is the case in which user sends a mutation. We display the response from server.
          that.setState({
            resType: 'success-res',
            response: JSON.stringify(result, null, 2)
          })
        } else if (key != undefined) {
          // We got the result for a query.
          var response = result[key]
          that.setState({
            'latency': result.server_latency.total
          });
          var startTime = new Date();
          for (var i = 0; i < response.length; i++) {
            processObject(response[i], {
              id: "",
              pred: key,
            })
          }
          renderNetwork(nodes, edges)
          var endTime = new Date();
          var timeTaken = (endTime.getTime() - startTime.getTime()) / 1000;
          that.setState({
            'rendering': timeTaken + 's'
          });
        } else {
          console.warn("We shouldn't be here really")
            // We probably didn't get any results.
          that.setState({
            resType: 'error-res',
            response: "Your query did not return any results."
          })
        }
      })).catch(function(error) {
      var err = error.response && error.response.text() || error.message
      return err
    }).then(function(errorMsg) {
      if (errorMsg != undefined) {
        that.setState({
          response: errorMsg,
          resType: 'error-res'
        })
      }
    })
  },
  render: function() {
    return (
      <div>
      <NavBar></NavBar>
    <div className="container">
        <div className="row justify-content-md-center">
            <div className="col-md-offset-1 col-md-10">
                <form id="query-form">
                    <div className="form-group">
                        <label htmlFor="query">Query</label>
                        <button type="submit" className="btn btn-primary pull-right" onClick={this.runQuery}>Run</button>
                    </div>
                </form>
      <div className="editor" style={editor}>
      <AceEditor
        mode="logiql"
        theme="github"
        name="editor"
        editorProps={{$blockScrolling: true}}
        width='100%'
        height='300px'
        fontSize='16px'
        value={this.query}
        focus={true}
        showPrintMargin={false}
        wrapEnabled={true}
        onChange={this.queryChange}
        commands={[{
          name: 'runQuery',
          bindKey: {
            win: 'Ctrl-Enter',
            mac: 'Command-Enter'
          },
          exec: function(editor) {
            this.runQuery(new Event(''));
          }.bind(this)
        }]}
      />
      </div>
          <label> Response </label>
          <div style={graph} id="graph" className={this.state.resType}>{this.state.response}</div>
          <div style={stats}>
              <form>
                      <div className="form-group col-sm-6">
                          <label htmlFor="server_latency">Server Latency</label>
                          <input type="input" className="form-control" value={this.state.latency} readOnly/>
                      </div>
                      <div className="form-group col-sm-6">
                          <label htmlFor="rendering">Rendering</label>
                          <input type="input" className="form-control" value={this.state.rendering} readOnly/>
                      </div>
              </form>
          </div>
        </div>
      </div>
    </div>
      </div>
    );
  }
});

module.exports = Home;
