import React from 'react';
import SessionItem from './SessionItem';
import CSSTransitionGroup from 'react-transition-group/CSSTransitionGroup'

import '../assets/css/SessionList.css';

const SessionList = ({ sessions }) => {
  return (
    <ul className="session-list">
      <CSSTransitionGroup
        transitionName="session-item"
        transitionEnterTimeout={800}
        transitionLeaveTimeout={300}
      >
        {
          sessions.map((session) => {
            return (
              <SessionItem
                key={session.id}
                session={session}
              />
            )
          })
        }
      </CSSTransitionGroup>
    </ul>
  );
};

export default SessionList;
