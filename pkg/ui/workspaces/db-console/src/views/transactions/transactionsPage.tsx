// Copyright 2018 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

import { connect } from "react-redux";
import { createSelector } from "reselect";
import { RouteComponentProps, withRouter } from "react-router-dom";
import { refreshStatements } from "src/redux/apiReducers";
import { resetSQLStatsAction } from "src/redux/sqlStats";
import { CachedDataReducerState } from "src/redux/cachedDataReducer";
import { AdminUIState } from "src/redux/state";
import { StatementsResponseMessage } from "src/util/api";

import { PrintTime } from "src/views/reports/containers/range/print";

import {
  Filters,
  defaultFilters,
  util,
  TransactionsPageStateProps,
  ActiveTransactionsViewDispatchProps,
  ActiveTransactionsViewStateProps,
  TransactionsPageDispatchProps,
  TransactionsPageRoot,
  TransactionsPageRootProps,
} from "@cockroachlabs/cluster-ui";
import { nodeRegionsByIDSelector } from "src/redux/nodes";
import { statementsTimeScaleLocalSetting } from "src/redux/statementsTimeScale";
import { setCombinedStatementsTimeScaleAction } from "src/redux/statements";
import { LocalSetting } from "src/redux/localsettings";
import { bindActionCreators } from "redux";
import {
  activeTransactionsPageActions,
  mapStateToActiveTransactionsPageProps,
} from "./activeTransactionsSelectors";

// selectStatements returns the array of AggregateStatistics to show on the
// TransactionsPage, based on if the appAttr route parameter is set.
export const selectData = createSelector(
  (state: AdminUIState) => state.cachedData.statements,
  (state: CachedDataReducerState<StatementsResponseMessage>) => {
    if (!state.data || state.inFlight || !state.valid) return null;
    return state.data;
  },
);

// selectLastReset returns a string displaying the last time the statement
// statistics were reset.
export const selectLastReset = createSelector(
  (state: AdminUIState) => state.cachedData.statements,
  (state: CachedDataReducerState<StatementsResponseMessage>) => {
    if (!state.data) {
      return "unknown";
    }

    return PrintTime(util.TimestampToMoment(state.data.last_reset));
  },
);

export const selectLastError = createSelector(
  (state: AdminUIState) => state.cachedData.statements,
  (state: CachedDataReducerState<StatementsResponseMessage>) => state.lastError,
);

export const sortSettingLocalSetting = new LocalSetting(
  "sortSetting/TransactionsPage",
  (state: AdminUIState) => state.localSettings,
  { ascending: false, columnTitle: "executionCount" },
);

export const filtersLocalSetting = new LocalSetting<AdminUIState, Filters>(
  "filters/TransactionsPage",
  (state: AdminUIState) => state.localSettings,
  defaultFilters,
);

export const searchLocalSetting = new LocalSetting(
  "search/TransactionsPage",
  (state: AdminUIState) => state.localSettings,
  null,
);

export const transactionColumnsLocalSetting = new LocalSetting(
  "showColumns/TransactionPage",
  (state: AdminUIState) => state.localSettings,
  null,
);

const fingerprintsPageActions = {
  refreshData: refreshStatements,
  resetSQLStats: resetSQLStatsAction,
  onTimeScaleChange: setCombinedStatementsTimeScaleAction,
  // We use `null` when the value was never set and it will show all columns.
  // If the user modifies the selection and no columns are selected,
  // the function will save the value as a blank space, otherwise
  // it gets saved as `null`.
  onColumnsChange: (value: string[]) =>
    transactionColumnsLocalSetting.set(
      value.length === 0 ? " " : value.join(","),
    ),
  onSortingChange: (
    _tableName: string,
    columnName: string,
    ascending: boolean,
  ) =>
    sortSettingLocalSetting.set({
      ascending: ascending,
      columnTitle: columnName,
    }),
  onFilterChange: (filters: Filters) => filtersLocalSetting.set(filters),
  onSearchComplete: (query: string) => searchLocalSetting.set(query),
};

type StateProps = {
  fingerprintsPageProps: TransactionsPageStateProps & RouteComponentProps;
  activePageProps: ActiveTransactionsViewStateProps;
};

type DispatchProps = {
  fingerprintsPageProps: TransactionsPageDispatchProps;
  activePageProps: ActiveTransactionsViewDispatchProps;
};

const TransactionsPageConnected = withRouter(
  connect<
    StateProps,
    DispatchProps,
    RouteComponentProps,
    TransactionsPageRootProps
  >(
    (state: AdminUIState, props: RouteComponentProps) => ({
      fingerprintsPageProps: {
        ...props,
        columns: transactionColumnsLocalSetting.selectorToArray(state),
        data: selectData(state),
        timeScale: statementsTimeScaleLocalSetting.selector(state),
        error: selectLastError(state),
        filters: filtersLocalSetting.selector(state),
        lastReset: selectLastReset(state),
        nodeRegions: nodeRegionsByIDSelector(state),
        search: searchLocalSetting.selector(state),
        sortSetting: sortSettingLocalSetting.selector(state),
        statementsError: state.cachedData.statements.lastError,
      },
      activePageProps: mapStateToActiveTransactionsPageProps(state),
    }),
    dispatch => ({
      fingerprintsPageProps: bindActionCreators(
        fingerprintsPageActions,
        dispatch,
      ),
      activePageProps: bindActionCreators(
        activeTransactionsPageActions,
        dispatch,
      ),
    }),
    (stateProps, dispatchProps) => ({
      fingerprintsPageProps: {
        ...stateProps.fingerprintsPageProps,
        ...dispatchProps.fingerprintsPageProps,
      },
      activePageProps: {
        ...stateProps.activePageProps,
        ...dispatchProps.activePageProps,
      },
    }),
  )(TransactionsPageRoot),
);

export default TransactionsPageConnected;
