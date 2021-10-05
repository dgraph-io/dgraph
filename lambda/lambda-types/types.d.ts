/*
 * Copyright 2021 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

declare module "@dgraph-lambda/lambda-types" {
  type GraphQLResponse = {
    data?: Record<string, any>,
    errors?: { message: string }[]
  }

  type AuthHeaderField = {
    key: string | undefined,
    value: string | undefined
  }

  type InfoField = {
    field: selectionField 
  }

  type selectionField = {
    alias: string,
    name: string,
    arguments: Record<string, any>,
    directives: fldDirectiveList,
    selectionSet: selectionSet
  }

  type selectionSet = Array<selectionField>

  type fldDirectiveList = Array<fldDirective>

  type fldDirective = {
    name: string,
    arguments: Record<string, any>
  }

  type eventPayload = {
    __typename: string,
    operation: string,
    commitTs: number,
    add: addEvent | undefined,
    update: updateEvent | undefined,
    delete: deleteEvent | undefined
  }


  type addEvent = {
    add: {
      rootUIDs: Array<any>,  
      input: Array<any>
    } 
  }

  type updateEvent = {
    update: {
      rootUIDs: Array<any>,
      SetPatch: Object,
      RemovePatch: Object
    } 
  }

  type deleteEvent = {
    delete: {
      rootUIDs: Array<any>
    } 
  }

  type GraphQLEventFields = {
    type: string,
    parents: (Record<string, any>)[] | null,
    args: Record<string, any>,
    authHeader?: AuthHeaderField,
    accessToken?: string, 
    event?: eventPayload,
    info?: InfoField
  }

  type ResolverResponse = any[] | Promise<any>[] | Promise<any[]>;

  type GraphQLEventCommonFields = {
    type: string;    
    respondWith: (r: ResolverResponse) => void;
    graphql: (s: string, vars: Record<string, any> | undefined, ah?: AuthHeaderField) => Promise<GraphQLResponse>;
    dql: {
      query: (s: string, vars: Record<string, any> | undefined) => Promise<GraphQLResponse>;
      mutate: (s: string) => Promise<GraphQLResponse>;
    };
    authHeader?: AuthHeaderField;
    accessToken?: string;
  };
  
  type GraphQLEvent = GraphQLEventCommonFields & {
    parents: Record<string, any>[] | null;
    args: Record<string, any>;
    info: InfoField;
  };
  
  type WebHookGraphQLEvent = GraphQLEventCommonFields & {
    event?: eventPayload;
  }; 

  type GraphQLEventWithParent = GraphQLEvent & {
    parent: Record<string, any> | null
  }

  function addGraphQLResolvers(resolvers: {
    [key: string]: (e: GraphQLEventWithParent) => any;
  }): void

  function addWebHookResolvers(resolvers: {
    [key: string]: (e: WebHookGraphQLEvent) => any;
  }): void

  function addMultiParentGraphQLResolvers(resolvers: {
    [key: string]: (e: GraphQLEvent) => ResolverResponse;
  }): void
}
