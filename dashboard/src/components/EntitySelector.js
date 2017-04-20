import React from "react";
import Label from "./Label";

import "../assets/css/EntitySelector.css";

const EntitySelector = ({
  response,
  onInitNodeTypeConfig,
  onUpdateLabelRegex,
  labelRegex,
  onUpdateLabels
}) => {
  return (
    <div className="entity-selector">
      <div className="row">
        <div className="col-xs-9">
          {response.plotAxis.map((label, i) => {
            return (
              <Label
                key={i}
                color={label.color}
                pred={label.pred}
                label={label.label}
                onInitNodeTypeConfig={onInitNodeTypeConfig}
              />
            );
          })}
        </div>
        <div className="col-xs-3">
          <div className="input-group">
            <input
              type="text"
              className="form-control"
              placeholder="Enter regex for labels"
              value={labelRegex}
              onChange={e => {
                onUpdateLabelRegex(e.target.value);
              }}
            />
            <span className="input-group-btn">
              <button
                className="btn btn-secondary"
                type="button"
                onClick={e => {
                  onUpdateLabels();
                }}
              >
                Done
              </button>
            </span>
          </div>
        </div>
      </div>
    </div>
  );
};
export default EntitySelector;
