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

function createRows(pieceArrays, props) {
    return pieceArrays.map((arr, i) => (
        <tr key={i}>
            {arr.map((key, j) => (
                <td key={j} className="Properties-key-val">
                    <div className="Properties-key">
                        {key}:&nbsp;
                    </div>
                    <div className="Properties-val">
                        {String(props[key])}
                    </div>
                </td>
            ))}
        </tr>
    ));
}

class Properties extends Component {
    render() {
        let props = JSON.parse(this.props.currentNode),
            // Nodes have facets and attrs keys.
            isEdge = Object.keys(props).length === 1,
            attrs = props["attrs"] || {},
            attrPieceArrays = getSubArrays(attrs),
            facets = props["facets"] || {},
            facetsPieceArrays = getSubArrays(facets);

        return (
            <div id="properties" style={{ marginTop: "5px" }}>
                {isEdge ? "Edge" : "Node"} Attributes:
                <table className="Properties">
                    <tbody>
                        {createRows(attrPieceArrays, attrs)}
                        {Object.keys(facets).length > 0 &&
                            <tr
                                style={{
                                    textAlign: "left",
                                }}
                            >
                                <td>{!isEdge && "Facets"}</td>
                            </tr>}
                        {Object.keys(facets).length > 0 &&
                            createRows(facetsPieceArrays, facets)}
                    </tbody>
                </table>
            </div>
        );
    }
}

export default Properties;
