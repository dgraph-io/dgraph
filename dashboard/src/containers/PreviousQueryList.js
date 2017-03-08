// @flow

import React, { Component } from "react";

import PreviousQuery from "./PreviousQuery";

import "../assets/css/App.css";

class PreviousQueryList extends Component {
  constructor(props) {
    super(props);
    this.state = {
      filterText: "",
    };
  }

  filterQueries = function() {
    var that = this;
    return this.props.queries.filter(function(item) {
      return item.text.toLowerCase().search(that.state.filterText) !== -1;
    });
  };

  updateFilterText = evt => {
    this.setState({
      filterText: evt.target.value,
    });
  };

  render() {
    return (
      <div className={`App-prev-queries ${this.props.xs}`}>
        <span><b>Previous Queries</b></span>
        <form style={{ marginTop: "5px" }}>
          <fieldset className="form-group">
            <input
              type="text"
              className="form-control"
              placeholder="Search"
              value={this.state.filterText}
              onChange={this.updateFilterText}
            />
          </fieldset>
        </form>
        <table className="App-prev-queries-table">
          <tbody className="App-prev-queries-tbody">
            {this.filterQueries().map(
              function(query, i) {
                return (
                  <PreviousQuery
                    text={query.text}
                    update={this.props.update}
                    delete={this.props.delete}
                    key={i}
                    idx={i}
                    lastRun={query.lastRun}
                    unique={i}
                  />
                );
              },
              this,
            )}
          </tbody>
        </table>
      </div>
    );
  }
}

export default PreviousQueryList;
