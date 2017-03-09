// @flow

import React, { Component } from "react";

import PreviousQuery from "../containers/PreviousQuery";

class PreviousQueryList extends Component {
    constructor(props: Props) {
        super(props);

        // TODO - See if we can get rid of this state and make this a dump component.
        this.state = {
            filterText: "",
        };
    }

    filterQueries = function(filterText) {
        return this.props.queries.filter(
            item =>
                item.text.toLowerCase().search(this.state.filterText) !== -1,
        );
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
                            onChange={e => {
                                this.setState({ filterText: e.target.value });
                            }}
                        />
                    </fieldset>
                </form>
                <table className="App-prev-queries-table">
                    <tbody className="App-prev-queries-tbody">
                        {this.filterQueries("").map(
                            function(query, i) {
                                return (
                                    <PreviousQuery
                                        text={query.text}
                                        lastRun={query.lastRun}
                                        key={i}
                                        idx={i}
                                        select={this.props.selectQuery}
                                        delete={this.props.deleteQuery}
                                        resetResponse={this.props.resetResponse}
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
