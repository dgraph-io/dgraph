// @flow

import React, { Component } from "react";

import "../assets/css/Properties.css";

function getSubArrays(obj) {
    let numRows = 3, pieceArrays = [], pieceArray = [];

    for (let k in obj) {
        if (!obj.hasOwnProperty(k)) {
            return;
        }
        pieceArray.push(k);
        if (pieceArray.length === numRows) {
            pieceArrays.push(pieceArray);
            pieceArray = [];
        }
    }
    if (pieceArray.length > 0) {
        pieceArrays.push(pieceArray);
    }
    return pieceArrays;
}

class Properties extends Component {
    render() {
        let props = JSON.parse(this.props.currentNode),
            pieceArrays = getSubArrays(props);

        return (
            <div id="properties" style={{ marginTop: "5px" }}>
                Current Node:
                <table className="Properties" title={this.props.currentNode}>
                    <tbody>
                        {pieceArrays.map((arr, i) => (
                            <tr key={i}>
                                {arr.map((key, j) => (
                                    <td key={j}>
                                        <span className="Properties-key">
                                            {key}
                                        </span>
                                        &nbsp;:{" "}
                                        <span className="Properties-val">
                                            {props[key]}
                                        </span>
                                    </td>
                                ))}
                                {arr.length !== 0 && arr.length < 3
                                    ? [...Array(3 - arr.length)].map((_, i) => {
                                          return <td key={i} />;
                                      })
                                    : []}
                            </tr>
                        ))}
                    </tbody>
                </table>
            </div>
        );
    }
}

export default Properties;
