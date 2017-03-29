// @flow

import React from "react";
import { Button } from "react-bootstrap";
import _ from "lodash/string";

import "../assets/css/Scratchpad.css";

function truncateName(name) {
    if (name === "") {
        return "_uid_";
    }
    if (name.length <= 10) {
        return name;
    }
    return _.truncate(name, { length: 10 });
}

function Scratchpad(props) {
    let { scratchpad, deleteAllEntries } = props;
    return (
        <div className="Scratchpad">
            <div className="Scratchpad-header">
                <h5 className="Scratchpad-heading"> Scratchpad </h5>
                <Button
                    className="Scratchpad-clear pull-right"
                    bsSize="xsmall"
                    onClick={() => {
                        deleteAllEntries();
                    }}
                >
                    Clear
                </Button>
            </div>
            <div className="Scratchpad-entries">
                {scratchpad.map((e, i) => (
                    <div className="Scratchpad-key-val" key={i}>
                        <div className="Scratchpad-key" title={e.name}>
                            {truncateName(e.name)}
                        </div>
                        {" "}
                        :
                        {" "}
                        <div className="Scratchpad-val">{e.uid}</div>
                    </div>
                ))}
            </div>
        </div>
    );
}

export default Scratchpad;
