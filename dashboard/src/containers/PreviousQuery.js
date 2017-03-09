// @flow

import React, { Component } from "react";
import { Popover, OverlayTrigger, Button, Glyphicon } from "react-bootstrap";

import "../assets/css/PreviousQuery.css";

function prettifyQuery(q: string) {
  var parsedQuery;
  try {
    parsedQuery = JSON.parse(q);
  } catch (e) {
    return q;
  }
  return JSON.stringify(parsedQuery, null, 2);
}

function getQueryStructure(query: string) {
  var lines = query.split("\n");
  lines = lines.map(function(line) {
    return line.trim();
  });
  var structure = "";
  for (var i = 0; i < lines.length; i++) {
    // We execute as soon as we encounter the first }
    if (lines[i].indexOf("}") !== -1) {
      break;
    }
    if (lines[i] === "{") {
      continue;
    }
    var openCurly = lines[i].indexOf("{");
    if (openCurly === -1) {
      continue;
    }
    var delim = " --> ";
    if (structure.length === 0) {
      delim = "";
    }
    structure = structure + delim + lines[i].substr(0, openCurly).trim();
  }
  return structure;
}

function plural(val) {
  if (val > 1) {
    return "s";
  } else {
    return "";
  }
}

function since(lastRun) {
  // In seconds
  let diff = (Date.now() - new Date(lastRun)) / 1000;

  let minute = 60,
    hour = minute * 60,
    day = hour * 24,
    // Lets just take 30.
    month = day * 30,
    year = month * 12;

  if (diff > year) {
    let val = Math.round(diff / year);
    return `${val} year${plural(val)} ago`;
  } else if (diff > month) {
    let val = Math.round(diff / month);
    return `${val} month${plural(val)} ago`;
  } else if (diff > day) {
    let val = Math.round(diff / day);
    return `${val} day${plural(val)} ago`;
  } else if (diff > hour) {
    let val = Math.round(diff / hour);
    return `${val} hour${plural(val)} ago`;
  } else if (diff > minute) {
    let val = Math.round(diff / minute);
    return `${val} minute${plural(val)} ago`;
  } else {
    let val = Math.round(diff / 1);
    return `${val} second${plural(val)} ago`;
  }
}

class PreviousQuery extends Component {
  render() {
    const popover = (
      <Popover id={this.props.idx}>
        <pre className="Previous-query-popover-pre">
          {this.props.text}
        </pre>
      </Popover>
    );

    const timeDiff = since(this.props.lastRun);
    return (
      <tr className="Previous-query">
        <td className="Previous-query-since">
          {timeDiff}
        </td>
        <td className="Previous-query-text">
          <OverlayTrigger
            delayShow={1500}
            delayHide={0}
            overlay={popover}
            placement="top"
          >
            <pre
              className="Previous-query-pre"
              onClick={() => {
                this.props.select(this.props.text);
                this.props.resetResponse();
              }}
              data-query={this.props.text}
            >
              {getQueryStructure(prettifyQuery(this.props.text))}
            </pre>
          </OverlayTrigger>
        </td>
        <td>
          <Button
            className="Previous-query-del"
            bsSize="xsmall"
            onClick={() => this.props.delete(this.props.idx)}
          >
            <Glyphicon glyph="remove" />
          </Button>
        </td>
      </tr>
    );
  }
}

export default PreviousQuery;
