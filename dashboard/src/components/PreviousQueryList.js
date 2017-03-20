// @flow

import React, { Component } from "react";
import { Button, InputGroup, FormControl } from "react-bootstrap";

import PreviousQuery from "../containers/PreviousQuery";

import "../assets/css/App.css";

class PreviousQueryList extends Component {
    constructor(props: Props) {
        super(props);

        // TODO - See if we can get rid of this state and make this a dumb component.
        this.state = {
            filterText: ""
        };
    }

    filterQueries = function(filterText) {
        let filter = filterText.toLowerCase();
        return this.props.queries.filter(
            item => item.text.toLowerCase().search(filter) !== -1
        );
    };
    render() {
        return (
            <div className={`App-prev-queries ${this.props.xs}`}>
                <div style={{ marginBottom: "5px" }}>
                    <span><b>Previous Queries</b></span>
                </div>
                <InputGroup bsSize="sm">
                    <FormControl
                        type="text"
                        placeholder="Search"
                        value={this.state.filterText}
                        onChange={e => {
                            this.setState({
                                filterText: e.target.value
                            });
                        }}
                    />
                    <InputGroup.Button>
                        <Button
                            bsStyle="danger"
                            onClick={() => this.props.deleteAll()}
                        >
                            Delete All
                        </Button>
                    </InputGroup.Button>
                </InputGroup>
                <table className="App-prev-queries-table">
                    <tbody className="App-prev-queries-tbody">
                        {this.filterQueries(
                            this.state.filterText
                        ).map((query, i) => {
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
                        })}
                    </tbody>
                </table>
            </div>
        );
    }
}

export default PreviousQueryList;
