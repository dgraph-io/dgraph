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

  if (structure === "") {
    return lines[0];
  }
  return structure;
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

    return (
      <tr className="Previous-query">
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
                window.scrollTo(0, 0);
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
