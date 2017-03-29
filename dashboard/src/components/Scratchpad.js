// @flow

import React from "react";
import { Button } from "react-bootstrap";

import "../assets/css/Scratchpad.css";

function truncateName(name) {
    if (name === "") {
        return "_uid_";
    }

    if (name.length <= 8) {
        return name;
    }
    return name.substr(0, 4) + "..." + name.substr(name.length - 4);
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
