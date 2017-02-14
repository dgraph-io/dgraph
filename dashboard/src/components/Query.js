import React, { Component } from 'react';
import {Popover, OverlayTrigger} from 'react-bootstrap';

function prettifyQuery(q) {
  var parsedQuery
  try {
    parsedQuery = JSON.parse(q);
  } catch (e) {
    return q;
  }
  return JSON.stringify(parsedQuery, null, 2);
}

function getQueryStructure(query) {
  var lines = query.split("\n");
  lines = lines.map(function(line) {
    return line.trim();
  })
  var structure = ""
  for (var i = 0; i < lines.length; i++) {
    // We execute as soon as we encounter the first }
    if (lines[i].indexOf("}") !== -1) {
      break;
    }
    if (lines[i] === "{") {
      continue
    }
    var openCurly = lines[i].indexOf("{");
    if (openCurly === -1) {
      continue
    }
    var delim = " --> "
    if (structure.length === 0) {
      delim = ""
    }
    structure = structure + delim + lines[i].substr(0, openCurly).trim()
  }
  return structure
}

class Query extends Component {
  render() {
const popover = (
  <Popover id={this.props.unique}>
    <pre style={{fontSize: '10px', whiteSpace: 'pre-wrap'}}>
    {this.props.text}
    </pre>
  </Popover>
);

    return (
      <div className="query" style={{marginBottom: '10px', padding: '5px', borderBottom: '1px solid gray'}}>
         <OverlayTrigger delayShow={1000} delayHide={0}
        overlay={popover} placement="bottom">
          <pre style={{whiteSpace: 'pre-wrap', backgroundColor: '#f0ece9'}}
            onClick={this.props.update} data-query={this.props.text}>{getQueryStructure(prettifyQuery(this.props.text))}
          </pre>
          </OverlayTrigger>
        <div className="col-sm-6">
        </div>
      </div>
    )
  }
}

export default Query;
