/*eslint-disable block-scoped-var, id-length, no-control-regex, no-magic-numbers, no-prototype-builtins, no-redeclare, no-shadow, no-var, sort-vars*/
import * as $protobuf from "protobufjs/minimal";

// Common aliases
const $Reader = $protobuf.Reader, $Writer = $protobuf.Writer, $util = $protobuf.util;

// Exported root namespace
const $root = $protobuf.roots["default"] || ($protobuf.roots["default"] = {});

export const proto = $root.proto = (() => {

    /**
     * Namespace proto.
     * @exports proto
     * @namespace
     */
    const proto = {};

    proto.Position = (function() {

        /**
         * Properties of a Position.
         * @memberof proto
         * @interface IPosition
         * @property {number|null} [x] Position x
         * @property {number|null} [y] Position y
         * @property {number|null} [heading] Position heading
         */

        /**
         * Constructs a new Position.
         * @memberof proto
         * @classdesc Represents a Position.
         * @implements IPosition
         * @constructor
         * @param {proto.IPosition=} [properties] Properties to set
         */
        function Position(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * Position x.
         * @member {number} x
         * @memberof proto.Position
         * @instance
         */
        Position.prototype.x = 0;

        /**
         * Position y.
         * @member {number} y
         * @memberof proto.Position
         * @instance
         */
        Position.prototype.y = 0;

        /**
         * Position heading.
         * @member {number} heading
         * @memberof proto.Position
         * @instance
         */
        Position.prototype.heading = 0;

        /**
         * Creates a new Position instance using the specified properties.
         * @function create
         * @memberof proto.Position
         * @static
         * @param {proto.IPosition=} [properties] Properties to set
         * @returns {proto.Position} Position instance
         */
        Position.create = function create(properties) {
            return new Position(properties);
        };

        /**
         * Encodes the specified Position message. Does not implicitly {@link proto.Position.verify|verify} messages.
         * @function encode
         * @memberof proto.Position
         * @static
         * @param {proto.IPosition} message Position message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Position.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.x != null && Object.hasOwnProperty.call(message, "x"))
                writer.uint32(/* id 1, wireType 0 =*/8).int32(message.x);
            if (message.y != null && Object.hasOwnProperty.call(message, "y"))
                writer.uint32(/* id 2, wireType 0 =*/16).int32(message.y);
            if (message.heading != null && Object.hasOwnProperty.call(message, "heading"))
                writer.uint32(/* id 3, wireType 0 =*/24).uint32(message.heading);
            return writer;
        };

        /**
         * Encodes the specified Position message, length delimited. Does not implicitly {@link proto.Position.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.Position
         * @static
         * @param {proto.IPosition} message Position message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Position.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a Position message from the specified reader or buffer.
         * @function decode
         * @memberof proto.Position
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.Position} Position
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Position.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.Position();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.x = reader.int32();
                        break;
                    }
                case 2: {
                        message.y = reader.int32();
                        break;
                    }
                case 3: {
                        message.heading = reader.uint32();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a Position message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.Position
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.Position} Position
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Position.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a Position message.
         * @function verify
         * @memberof proto.Position
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        Position.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.x != null && message.hasOwnProperty("x"))
                if (!$util.isInteger(message.x))
                    return "x: integer expected";
            if (message.y != null && message.hasOwnProperty("y"))
                if (!$util.isInteger(message.y))
                    return "y: integer expected";
            if (message.heading != null && message.hasOwnProperty("heading"))
                if (!$util.isInteger(message.heading))
                    return "heading: integer expected";
            return null;
        };

        /**
         * Creates a Position message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.Position
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.Position} Position
         */
        Position.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.Position)
                return object;
            let message = new $root.proto.Position();
            if (object.x != null)
                message.x = object.x | 0;
            if (object.y != null)
                message.y = object.y | 0;
            if (object.heading != null)
                message.heading = object.heading >>> 0;
            return message;
        };

        /**
         * Creates a plain object from a Position message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.Position
         * @static
         * @param {proto.Position} message Position
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        Position.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.x = 0;
                object.y = 0;
                object.heading = 0;
            }
            if (message.x != null && message.hasOwnProperty("x"))
                object.x = message.x;
            if (message.y != null && message.hasOwnProperty("y"))
                object.y = message.y;
            if (message.heading != null && message.hasOwnProperty("heading"))
                object.heading = message.heading;
            return object;
        };

        /**
         * Converts this Position to JSON.
         * @function toJSON
         * @memberof proto.Position
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        Position.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for Position
         * @function getTypeUrl
         * @memberof proto.Position
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        Position.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.Position";
        };

        return Position;
    })();

    proto.Vector2 = (function() {

        /**
         * Properties of a Vector2.
         * @memberof proto
         * @interface IVector2
         * @property {number|null} [x] Vector2 x
         * @property {number|null} [y] Vector2 y
         */

        /**
         * Constructs a new Vector2.
         * @memberof proto
         * @classdesc Represents a Vector2.
         * @implements IVector2
         * @constructor
         * @param {proto.IVector2=} [properties] Properties to set
         */
        function Vector2(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * Vector2 x.
         * @member {number} x
         * @memberof proto.Vector2
         * @instance
         */
        Vector2.prototype.x = 0;

        /**
         * Vector2 y.
         * @member {number} y
         * @memberof proto.Vector2
         * @instance
         */
        Vector2.prototype.y = 0;

        /**
         * Creates a new Vector2 instance using the specified properties.
         * @function create
         * @memberof proto.Vector2
         * @static
         * @param {proto.IVector2=} [properties] Properties to set
         * @returns {proto.Vector2} Vector2 instance
         */
        Vector2.create = function create(properties) {
            return new Vector2(properties);
        };

        /**
         * Encodes the specified Vector2 message. Does not implicitly {@link proto.Vector2.verify|verify} messages.
         * @function encode
         * @memberof proto.Vector2
         * @static
         * @param {proto.IVector2} message Vector2 message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Vector2.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.x != null && Object.hasOwnProperty.call(message, "x"))
                writer.uint32(/* id 1, wireType 0 =*/8).int32(message.x);
            if (message.y != null && Object.hasOwnProperty.call(message, "y"))
                writer.uint32(/* id 2, wireType 0 =*/16).int32(message.y);
            return writer;
        };

        /**
         * Encodes the specified Vector2 message, length delimited. Does not implicitly {@link proto.Vector2.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.Vector2
         * @static
         * @param {proto.IVector2} message Vector2 message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Vector2.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a Vector2 message from the specified reader or buffer.
         * @function decode
         * @memberof proto.Vector2
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.Vector2} Vector2
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Vector2.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.Vector2();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.x = reader.int32();
                        break;
                    }
                case 2: {
                        message.y = reader.int32();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a Vector2 message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.Vector2
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.Vector2} Vector2
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Vector2.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a Vector2 message.
         * @function verify
         * @memberof proto.Vector2
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        Vector2.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.x != null && message.hasOwnProperty("x"))
                if (!$util.isInteger(message.x))
                    return "x: integer expected";
            if (message.y != null && message.hasOwnProperty("y"))
                if (!$util.isInteger(message.y))
                    return "y: integer expected";
            return null;
        };

        /**
         * Creates a Vector2 message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.Vector2
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.Vector2} Vector2
         */
        Vector2.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.Vector2)
                return object;
            let message = new $root.proto.Vector2();
            if (object.x != null)
                message.x = object.x | 0;
            if (object.y != null)
                message.y = object.y | 0;
            return message;
        };

        /**
         * Creates a plain object from a Vector2 message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.Vector2
         * @static
         * @param {proto.Vector2} message Vector2
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        Vector2.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.x = 0;
                object.y = 0;
            }
            if (message.x != null && message.hasOwnProperty("x"))
                object.x = message.x;
            if (message.y != null && message.hasOwnProperty("y"))
                object.y = message.y;
            return object;
        };

        /**
         * Converts this Vector2 to JSON.
         * @function toJSON
         * @memberof proto.Vector2
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        Vector2.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for Vector2
         * @function getTypeUrl
         * @memberof proto.Vector2
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        Vector2.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.Vector2";
        };

        return Vector2;
    })();

    proto.AABB = (function() {

        /**
         * Properties of a AABB.
         * @memberof proto
         * @interface IAABB
         * @property {number|null} [minX] AABB minX
         * @property {number|null} [minY] AABB minY
         * @property {number|null} [maxX] AABB maxX
         * @property {number|null} [maxY] AABB maxY
         */

        /**
         * Constructs a new AABB.
         * @memberof proto
         * @classdesc Represents a AABB.
         * @implements IAABB
         * @constructor
         * @param {proto.IAABB=} [properties] Properties to set
         */
        function AABB(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * AABB minX.
         * @member {number} minX
         * @memberof proto.AABB
         * @instance
         */
        AABB.prototype.minX = 0;

        /**
         * AABB minY.
         * @member {number} minY
         * @memberof proto.AABB
         * @instance
         */
        AABB.prototype.minY = 0;

        /**
         * AABB maxX.
         * @member {number} maxX
         * @memberof proto.AABB
         * @instance
         */
        AABB.prototype.maxX = 0;

        /**
         * AABB maxY.
         * @member {number} maxY
         * @memberof proto.AABB
         * @instance
         */
        AABB.prototype.maxY = 0;

        /**
         * Creates a new AABB instance using the specified properties.
         * @function create
         * @memberof proto.AABB
         * @static
         * @param {proto.IAABB=} [properties] Properties to set
         * @returns {proto.AABB} AABB instance
         */
        AABB.create = function create(properties) {
            return new AABB(properties);
        };

        /**
         * Encodes the specified AABB message. Does not implicitly {@link proto.AABB.verify|verify} messages.
         * @function encode
         * @memberof proto.AABB
         * @static
         * @param {proto.IAABB} message AABB message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        AABB.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.minX != null && Object.hasOwnProperty.call(message, "minX"))
                writer.uint32(/* id 1, wireType 0 =*/8).int32(message.minX);
            if (message.minY != null && Object.hasOwnProperty.call(message, "minY"))
                writer.uint32(/* id 2, wireType 0 =*/16).int32(message.minY);
            if (message.maxX != null && Object.hasOwnProperty.call(message, "maxX"))
                writer.uint32(/* id 3, wireType 0 =*/24).int32(message.maxX);
            if (message.maxY != null && Object.hasOwnProperty.call(message, "maxY"))
                writer.uint32(/* id 4, wireType 0 =*/32).int32(message.maxY);
            return writer;
        };

        /**
         * Encodes the specified AABB message, length delimited. Does not implicitly {@link proto.AABB.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.AABB
         * @static
         * @param {proto.IAABB} message AABB message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        AABB.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a AABB message from the specified reader or buffer.
         * @function decode
         * @memberof proto.AABB
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.AABB} AABB
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        AABB.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.AABB();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.minX = reader.int32();
                        break;
                    }
                case 2: {
                        message.minY = reader.int32();
                        break;
                    }
                case 3: {
                        message.maxX = reader.int32();
                        break;
                    }
                case 4: {
                        message.maxY = reader.int32();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a AABB message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.AABB
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.AABB} AABB
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        AABB.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a AABB message.
         * @function verify
         * @memberof proto.AABB
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        AABB.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.minX != null && message.hasOwnProperty("minX"))
                if (!$util.isInteger(message.minX))
                    return "minX: integer expected";
            if (message.minY != null && message.hasOwnProperty("minY"))
                if (!$util.isInteger(message.minY))
                    return "minY: integer expected";
            if (message.maxX != null && message.hasOwnProperty("maxX"))
                if (!$util.isInteger(message.maxX))
                    return "maxX: integer expected";
            if (message.maxY != null && message.hasOwnProperty("maxY"))
                if (!$util.isInteger(message.maxY))
                    return "maxY: integer expected";
            return null;
        };

        /**
         * Creates a AABB message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.AABB
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.AABB} AABB
         */
        AABB.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.AABB)
                return object;
            let message = new $root.proto.AABB();
            if (object.minX != null)
                message.minX = object.minX | 0;
            if (object.minY != null)
                message.minY = object.minY | 0;
            if (object.maxX != null)
                message.maxX = object.maxX | 0;
            if (object.maxY != null)
                message.maxY = object.maxY | 0;
            return message;
        };

        /**
         * Creates a plain object from a AABB message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.AABB
         * @static
         * @param {proto.AABB} message AABB
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        AABB.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.minX = 0;
                object.minY = 0;
                object.maxX = 0;
                object.maxY = 0;
            }
            if (message.minX != null && message.hasOwnProperty("minX"))
                object.minX = message.minX;
            if (message.minY != null && message.hasOwnProperty("minY"))
                object.minY = message.minY;
            if (message.maxX != null && message.hasOwnProperty("maxX"))
                object.maxX = message.maxX;
            if (message.maxY != null && message.hasOwnProperty("maxY"))
                object.maxY = message.maxY;
            return object;
        };

        /**
         * Converts this AABB to JSON.
         * @function toJSON
         * @memberof proto.AABB
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        AABB.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for AABB
         * @function getTypeUrl
         * @memberof proto.AABB
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        AABB.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.AABB";
        };

        return AABB;
    })();

    proto.Timestamp = (function() {

        /**
         * Properties of a Timestamp.
         * @memberof proto
         * @interface ITimestamp
         * @property {number|Long|null} [millis] Timestamp millis
         */

        /**
         * Constructs a new Timestamp.
         * @memberof proto
         * @classdesc Represents a Timestamp.
         * @implements ITimestamp
         * @constructor
         * @param {proto.ITimestamp=} [properties] Properties to set
         */
        function Timestamp(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * Timestamp millis.
         * @member {number|Long} millis
         * @memberof proto.Timestamp
         * @instance
         */
        Timestamp.prototype.millis = $util.Long ? $util.Long.fromBits(0,0,false) : 0;

        /**
         * Creates a new Timestamp instance using the specified properties.
         * @function create
         * @memberof proto.Timestamp
         * @static
         * @param {proto.ITimestamp=} [properties] Properties to set
         * @returns {proto.Timestamp} Timestamp instance
         */
        Timestamp.create = function create(properties) {
            return new Timestamp(properties);
        };

        /**
         * Encodes the specified Timestamp message. Does not implicitly {@link proto.Timestamp.verify|verify} messages.
         * @function encode
         * @memberof proto.Timestamp
         * @static
         * @param {proto.ITimestamp} message Timestamp message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Timestamp.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.millis != null && Object.hasOwnProperty.call(message, "millis"))
                writer.uint32(/* id 1, wireType 0 =*/8).int64(message.millis);
            return writer;
        };

        /**
         * Encodes the specified Timestamp message, length delimited. Does not implicitly {@link proto.Timestamp.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.Timestamp
         * @static
         * @param {proto.ITimestamp} message Timestamp message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Timestamp.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a Timestamp message from the specified reader or buffer.
         * @function decode
         * @memberof proto.Timestamp
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.Timestamp} Timestamp
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Timestamp.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.Timestamp();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.millis = reader.int64();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a Timestamp message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.Timestamp
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.Timestamp} Timestamp
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Timestamp.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a Timestamp message.
         * @function verify
         * @memberof proto.Timestamp
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        Timestamp.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.millis != null && message.hasOwnProperty("millis"))
                if (!$util.isInteger(message.millis) && !(message.millis && $util.isInteger(message.millis.low) && $util.isInteger(message.millis.high)))
                    return "millis: integer|Long expected";
            return null;
        };

        /**
         * Creates a Timestamp message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.Timestamp
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.Timestamp} Timestamp
         */
        Timestamp.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.Timestamp)
                return object;
            let message = new $root.proto.Timestamp();
            if (object.millis != null)
                if ($util.Long)
                    (message.millis = $util.Long.fromValue(object.millis)).unsigned = false;
                else if (typeof object.millis === "string")
                    message.millis = parseInt(object.millis, 10);
                else if (typeof object.millis === "number")
                    message.millis = object.millis;
                else if (typeof object.millis === "object")
                    message.millis = new $util.LongBits(object.millis.low >>> 0, object.millis.high >>> 0).toNumber();
            return message;
        };

        /**
         * Creates a plain object from a Timestamp message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.Timestamp
         * @static
         * @param {proto.Timestamp} message Timestamp
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        Timestamp.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults)
                if ($util.Long) {
                    let long = new $util.Long(0, 0, false);
                    object.millis = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.millis = options.longs === String ? "0" : 0;
            if (message.millis != null && message.hasOwnProperty("millis"))
                if (typeof message.millis === "number")
                    object.millis = options.longs === String ? String(message.millis) : message.millis;
                else
                    object.millis = options.longs === String ? $util.Long.prototype.toString.call(message.millis) : options.longs === Number ? new $util.LongBits(message.millis.low >>> 0, message.millis.high >>> 0).toNumber() : message.millis;
            return object;
        };

        /**
         * Converts this Timestamp to JSON.
         * @function toJSON
         * @memberof proto.Timestamp
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        Timestamp.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for Timestamp
         * @function getTypeUrl
         * @memberof proto.Timestamp
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        Timestamp.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.Timestamp";
        };

        return Timestamp;
    })();

    /**
     * MovementMode enum.
     * @name proto.MovementMode
     * @enum {number}
     * @property {number} MOVE_MODE_WALK=0 MOVE_MODE_WALK value
     * @property {number} MOVE_MODE_RUN=1 MOVE_MODE_RUN value
     * @property {number} MOVE_MODE_FAST_RUN=2 MOVE_MODE_FAST_RUN value
     * @property {number} MOVE_MODE_SWIM=3 MOVE_MODE_SWIM value
     */
    proto.MovementMode = (function() {
        const valuesById = {}, values = Object.create(valuesById);
        values[valuesById[0] = "MOVE_MODE_WALK"] = 0;
        values[valuesById[1] = "MOVE_MODE_RUN"] = 1;
        values[valuesById[2] = "MOVE_MODE_FAST_RUN"] = 2;
        values[valuesById[3] = "MOVE_MODE_SWIM"] = 3;
        return values;
    })();

    /**
     * EquipSlot enum.
     * @name proto.EquipSlot
     * @enum {number}
     * @property {number} EQUIP_SLOT_NONE=0 EQUIP_SLOT_NONE value
     * @property {number} EQUIP_SLOT_HEAD=1 EQUIP_SLOT_HEAD value
     * @property {number} EQUIP_SLOT_CHEST=2 EQUIP_SLOT_CHEST value
     * @property {number} EQUIP_SLOT_LEGS=3 EQUIP_SLOT_LEGS value
     * @property {number} EQUIP_SLOT_FEET=4 EQUIP_SLOT_FEET value
     * @property {number} EQUIP_SLOT_HANDS=5 EQUIP_SLOT_HANDS value
     * @property {number} EQUIP_SLOT_LEFT_HAND=6 EQUIP_SLOT_LEFT_HAND value
     * @property {number} EQUIP_SLOT_RIGHT_HAND=7 EQUIP_SLOT_RIGHT_HAND value
     * @property {number} EQUIP_SLOT_BACK=8 EQUIP_SLOT_BACK value
     * @property {number} EQUIP_SLOT_NECK=9 EQUIP_SLOT_NECK value
     * @property {number} EQUIP_SLOT_RING_1=10 EQUIP_SLOT_RING_1 value
     * @property {number} EQUIP_SLOT_RING_2=11 EQUIP_SLOT_RING_2 value
     */
    proto.EquipSlot = (function() {
        const valuesById = {}, values = Object.create(valuesById);
        values[valuesById[0] = "EQUIP_SLOT_NONE"] = 0;
        values[valuesById[1] = "EQUIP_SLOT_HEAD"] = 1;
        values[valuesById[2] = "EQUIP_SLOT_CHEST"] = 2;
        values[valuesById[3] = "EQUIP_SLOT_LEGS"] = 3;
        values[valuesById[4] = "EQUIP_SLOT_FEET"] = 4;
        values[valuesById[5] = "EQUIP_SLOT_HANDS"] = 5;
        values[valuesById[6] = "EQUIP_SLOT_LEFT_HAND"] = 6;
        values[valuesById[7] = "EQUIP_SLOT_RIGHT_HAND"] = 7;
        values[valuesById[8] = "EQUIP_SLOT_BACK"] = 8;
        values[valuesById[9] = "EQUIP_SLOT_NECK"] = 9;
        values[valuesById[10] = "EQUIP_SLOT_RING_1"] = 10;
        values[valuesById[11] = "EQUIP_SLOT_RING_2"] = 11;
        return values;
    })();

    /**
     * ExpType enum.
     * @name proto.ExpType
     * @enum {number}
     * @property {number} EXP_TYPE_NATURE=0 EXP_TYPE_NATURE value
     * @property {number} EXP_TYPE_INDUSTRY=1 EXP_TYPE_INDUSTRY value
     * @property {number} EXP_TYPE_COMBAT=2 EXP_TYPE_COMBAT value
     */
    proto.ExpType = (function() {
        const valuesById = {}, values = Object.create(valuesById);
        values[valuesById[0] = "EXP_TYPE_NATURE"] = 0;
        values[valuesById[1] = "EXP_TYPE_INDUSTRY"] = 1;
        values[valuesById[2] = "EXP_TYPE_COMBAT"] = 2;
        return values;
    })();

    /**
     * WeatherType enum.
     * @name proto.WeatherType
     * @enum {number}
     * @property {number} WEATHER_TYPE_CLEAR=0 WEATHER_TYPE_CLEAR value
     * @property {number} WEATHER_TYPE_RAIN=1 WEATHER_TYPE_RAIN value
     * @property {number} WEATHER_TYPE_FOG=2 WEATHER_TYPE_FOG value
     * @property {number} WEATHER_TYPE_STORM=3 WEATHER_TYPE_STORM value
     * @property {number} WEATHER_TYPE_SNOW=4 WEATHER_TYPE_SNOW value
     */
    proto.WeatherType = (function() {
        const valuesById = {}, values = Object.create(valuesById);
        values[valuesById[0] = "WEATHER_TYPE_CLEAR"] = 0;
        values[valuesById[1] = "WEATHER_TYPE_RAIN"] = 1;
        values[valuesById[2] = "WEATHER_TYPE_FOG"] = 2;
        values[valuesById[3] = "WEATHER_TYPE_STORM"] = 3;
        values[valuesById[4] = "WEATHER_TYPE_SNOW"] = 4;
        return values;
    })();

    /**
     * InventoryKind enum.
     * @name proto.InventoryKind
     * @enum {number}
     * @property {number} INVENTORY_KIND_GRID=0 INVENTORY_KIND_GRID value
     * @property {number} INVENTORY_KIND_HAND=1 INVENTORY_KIND_HAND value
     * @property {number} INVENTORY_KIND_EQUIPMENT=2 INVENTORY_KIND_EQUIPMENT value
     * @property {number} INVENTORY_KIND_DROPPED_ITEM=3 INVENTORY_KIND_DROPPED_ITEM value
     */
    proto.InventoryKind = (function() {
        const valuesById = {}, values = Object.create(valuesById);
        values[valuesById[0] = "INVENTORY_KIND_GRID"] = 0;
        values[valuesById[1] = "INVENTORY_KIND_HAND"] = 1;
        values[valuesById[2] = "INVENTORY_KIND_EQUIPMENT"] = 2;
        values[valuesById[3] = "INVENTORY_KIND_DROPPED_ITEM"] = 3;
        return values;
    })();

    /**
     * ErrorCode enum.
     * @name proto.ErrorCode
     * @enum {number}
     * @property {number} ERROR_CODE_NONE=0 ERROR_CODE_NONE value
     * @property {number} ERROR_CODE_INVALID_REQUEST=1 ERROR_CODE_INVALID_REQUEST value
     * @property {number} ERROR_CODE_NOT_AUTHENTICATED=2 ERROR_CODE_NOT_AUTHENTICATED value
     * @property {number} ERROR_CODE_ENTITY_NOT_FOUND=3 ERROR_CODE_ENTITY_NOT_FOUND value
     * @property {number} ERROR_CODE_OUT_OF_RANGE=4 ERROR_CODE_OUT_OF_RANGE value
     * @property {number} ERROR_CODE_INSUFFICIENT_RESOURCES=5 ERROR_CODE_INSUFFICIENT_RESOURCES value
     * @property {number} ERROR_CODE_INVENTORY_FULL=6 ERROR_CODE_INVENTORY_FULL value
     * @property {number} ERROR_CODE_CANNOT_INTERACT=7 ERROR_CODE_CANNOT_INTERACT value
     * @property {number} ERROR_CODE_COOLDOWN_ACTIVE=8 ERROR_CODE_COOLDOWN_ACTIVE value
     * @property {number} ERROR_CODE_INSUFFICIENT_STAMINA=9 ERROR_CODE_INSUFFICIENT_STAMINA value
     * @property {number} ERROR_CODE_TARGET_INVALID=10 ERROR_CODE_TARGET_INVALID value
     * @property {number} ERROR_CODE_PATH_BLOCKED=11 ERROR_CODE_PATH_BLOCKED value
     * @property {number} ERROR_CODE_TIMEOUT_EXCEEDED=12 ERROR_CODE_TIMEOUT_EXCEEDED value
     * @property {number} ERROR_CODE_BUILDING_INCOMPLETE=13 ERROR_CODE_BUILDING_INCOMPLETE value
     * @property {number} ERROR_CODE_RECIPE_UNKNOWN=14 ERROR_CODE_RECIPE_UNKNOWN value
     * @property {number} ERROR_PACKET_PER_SECOND_LIMIT_THRESHOLDED=15 ERROR_PACKET_PER_SECOND_LIMIT_THRESHOLDED value
     * @property {number} ERROR_CODE_INTERNAL_ERROR=16 ERROR_CODE_INTERNAL_ERROR value
     */
    proto.ErrorCode = (function() {
        const valuesById = {}, values = Object.create(valuesById);
        values[valuesById[0] = "ERROR_CODE_NONE"] = 0;
        values[valuesById[1] = "ERROR_CODE_INVALID_REQUEST"] = 1;
        values[valuesById[2] = "ERROR_CODE_NOT_AUTHENTICATED"] = 2;
        values[valuesById[3] = "ERROR_CODE_ENTITY_NOT_FOUND"] = 3;
        values[valuesById[4] = "ERROR_CODE_OUT_OF_RANGE"] = 4;
        values[valuesById[5] = "ERROR_CODE_INSUFFICIENT_RESOURCES"] = 5;
        values[valuesById[6] = "ERROR_CODE_INVENTORY_FULL"] = 6;
        values[valuesById[7] = "ERROR_CODE_CANNOT_INTERACT"] = 7;
        values[valuesById[8] = "ERROR_CODE_COOLDOWN_ACTIVE"] = 8;
        values[valuesById[9] = "ERROR_CODE_INSUFFICIENT_STAMINA"] = 9;
        values[valuesById[10] = "ERROR_CODE_TARGET_INVALID"] = 10;
        values[valuesById[11] = "ERROR_CODE_PATH_BLOCKED"] = 11;
        values[valuesById[12] = "ERROR_CODE_TIMEOUT_EXCEEDED"] = 12;
        values[valuesById[13] = "ERROR_CODE_BUILDING_INCOMPLETE"] = 13;
        values[valuesById[14] = "ERROR_CODE_RECIPE_UNKNOWN"] = 14;
        values[valuesById[15] = "ERROR_PACKET_PER_SECOND_LIMIT_THRESHOLDED"] = 15;
        values[valuesById[16] = "ERROR_CODE_INTERNAL_ERROR"] = 16;
        return values;
    })();

    /**
     * WarningCode enum.
     * @name proto.WarningCode
     * @enum {number}
     * @property {number} WARN_INPUT_QUEUE_OVERFLOW=0 WARN_INPUT_QUEUE_OVERFLOW value
     */
    proto.WarningCode = (function() {
        const valuesById = {}, values = Object.create(valuesById);
        values[valuesById[0] = "WARN_INPUT_QUEUE_OVERFLOW"] = 0;
        return values;
    })();

    proto.InventoryRef = (function() {

        /**
         * Properties of an InventoryRef.
         * @memberof proto
         * @interface IInventoryRef
         * @property {proto.InventoryKind|null} [kind] InventoryRef kind
         * @property {number|Long|null} [ownerEntityId] InventoryRef ownerEntityId
         * @property {number|null} [inventoryKey] InventoryRef inventoryKey
         */

        /**
         * Constructs a new InventoryRef.
         * @memberof proto
         * @classdesc Represents an InventoryRef.
         * @implements IInventoryRef
         * @constructor
         * @param {proto.IInventoryRef=} [properties] Properties to set
         */
        function InventoryRef(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * InventoryRef kind.
         * @member {proto.InventoryKind} kind
         * @memberof proto.InventoryRef
         * @instance
         */
        InventoryRef.prototype.kind = 0;

        /**
         * InventoryRef ownerEntityId.
         * @member {number|Long} ownerEntityId
         * @memberof proto.InventoryRef
         * @instance
         */
        InventoryRef.prototype.ownerEntityId = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * InventoryRef inventoryKey.
         * @member {number} inventoryKey
         * @memberof proto.InventoryRef
         * @instance
         */
        InventoryRef.prototype.inventoryKey = 0;

        /**
         * Creates a new InventoryRef instance using the specified properties.
         * @function create
         * @memberof proto.InventoryRef
         * @static
         * @param {proto.IInventoryRef=} [properties] Properties to set
         * @returns {proto.InventoryRef} InventoryRef instance
         */
        InventoryRef.create = function create(properties) {
            return new InventoryRef(properties);
        };

        /**
         * Encodes the specified InventoryRef message. Does not implicitly {@link proto.InventoryRef.verify|verify} messages.
         * @function encode
         * @memberof proto.InventoryRef
         * @static
         * @param {proto.IInventoryRef} message InventoryRef message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        InventoryRef.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.kind != null && Object.hasOwnProperty.call(message, "kind"))
                writer.uint32(/* id 1, wireType 0 =*/8).int32(message.kind);
            if (message.ownerEntityId != null && Object.hasOwnProperty.call(message, "ownerEntityId"))
                writer.uint32(/* id 2, wireType 0 =*/16).uint64(message.ownerEntityId);
            if (message.inventoryKey != null && Object.hasOwnProperty.call(message, "inventoryKey"))
                writer.uint32(/* id 3, wireType 0 =*/24).uint32(message.inventoryKey);
            return writer;
        };

        /**
         * Encodes the specified InventoryRef message, length delimited. Does not implicitly {@link proto.InventoryRef.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.InventoryRef
         * @static
         * @param {proto.IInventoryRef} message InventoryRef message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        InventoryRef.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes an InventoryRef message from the specified reader or buffer.
         * @function decode
         * @memberof proto.InventoryRef
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.InventoryRef} InventoryRef
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        InventoryRef.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.InventoryRef();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.kind = reader.int32();
                        break;
                    }
                case 2: {
                        message.ownerEntityId = reader.uint64();
                        break;
                    }
                case 3: {
                        message.inventoryKey = reader.uint32();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes an InventoryRef message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.InventoryRef
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.InventoryRef} InventoryRef
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        InventoryRef.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies an InventoryRef message.
         * @function verify
         * @memberof proto.InventoryRef
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        InventoryRef.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.kind != null && message.hasOwnProperty("kind"))
                switch (message.kind) {
                default:
                    return "kind: enum value expected";
                case 0:
                case 1:
                case 2:
                case 3:
                    break;
                }
            if (message.ownerEntityId != null && message.hasOwnProperty("ownerEntityId"))
                if (!$util.isInteger(message.ownerEntityId) && !(message.ownerEntityId && $util.isInteger(message.ownerEntityId.low) && $util.isInteger(message.ownerEntityId.high)))
                    return "ownerEntityId: integer|Long expected";
            if (message.inventoryKey != null && message.hasOwnProperty("inventoryKey"))
                if (!$util.isInteger(message.inventoryKey))
                    return "inventoryKey: integer expected";
            return null;
        };

        /**
         * Creates an InventoryRef message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.InventoryRef
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.InventoryRef} InventoryRef
         */
        InventoryRef.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.InventoryRef)
                return object;
            let message = new $root.proto.InventoryRef();
            switch (object.kind) {
            default:
                if (typeof object.kind === "number") {
                    message.kind = object.kind;
                    break;
                }
                break;
            case "INVENTORY_KIND_GRID":
            case 0:
                message.kind = 0;
                break;
            case "INVENTORY_KIND_HAND":
            case 1:
                message.kind = 1;
                break;
            case "INVENTORY_KIND_EQUIPMENT":
            case 2:
                message.kind = 2;
                break;
            case "INVENTORY_KIND_DROPPED_ITEM":
            case 3:
                message.kind = 3;
                break;
            }
            if (object.ownerEntityId != null)
                if ($util.Long)
                    (message.ownerEntityId = $util.Long.fromValue(object.ownerEntityId)).unsigned = true;
                else if (typeof object.ownerEntityId === "string")
                    message.ownerEntityId = parseInt(object.ownerEntityId, 10);
                else if (typeof object.ownerEntityId === "number")
                    message.ownerEntityId = object.ownerEntityId;
                else if (typeof object.ownerEntityId === "object")
                    message.ownerEntityId = new $util.LongBits(object.ownerEntityId.low >>> 0, object.ownerEntityId.high >>> 0).toNumber(true);
            if (object.inventoryKey != null)
                message.inventoryKey = object.inventoryKey >>> 0;
            return message;
        };

        /**
         * Creates a plain object from an InventoryRef message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.InventoryRef
         * @static
         * @param {proto.InventoryRef} message InventoryRef
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        InventoryRef.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.kind = options.enums === String ? "INVENTORY_KIND_GRID" : 0;
                if ($util.Long) {
                    let long = new $util.Long(0, 0, true);
                    object.ownerEntityId = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.ownerEntityId = options.longs === String ? "0" : 0;
                object.inventoryKey = 0;
            }
            if (message.kind != null && message.hasOwnProperty("kind"))
                object.kind = options.enums === String ? $root.proto.InventoryKind[message.kind] === undefined ? message.kind : $root.proto.InventoryKind[message.kind] : message.kind;
            if (message.ownerEntityId != null && message.hasOwnProperty("ownerEntityId"))
                if (typeof message.ownerEntityId === "number")
                    object.ownerEntityId = options.longs === String ? String(message.ownerEntityId) : message.ownerEntityId;
                else
                    object.ownerEntityId = options.longs === String ? $util.Long.prototype.toString.call(message.ownerEntityId) : options.longs === Number ? new $util.LongBits(message.ownerEntityId.low >>> 0, message.ownerEntityId.high >>> 0).toNumber(true) : message.ownerEntityId;
            if (message.inventoryKey != null && message.hasOwnProperty("inventoryKey"))
                object.inventoryKey = message.inventoryKey;
            return object;
        };

        /**
         * Converts this InventoryRef to JSON.
         * @function toJSON
         * @memberof proto.InventoryRef
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        InventoryRef.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for InventoryRef
         * @function getTypeUrl
         * @memberof proto.InventoryRef
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        InventoryRef.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.InventoryRef";
        };

        return InventoryRef;
    })();

    proto.ItemInstance = (function() {

        /**
         * Properties of an ItemInstance.
         * @memberof proto
         * @interface IItemInstance
         * @property {number|Long|null} [itemId] ItemInstance itemId
         * @property {number|null} [typeId] ItemInstance typeId
         * @property {string|null} [resource] ItemInstance resource
         * @property {number|null} [quality] ItemInstance quality
         * @property {number|null} [quantity] ItemInstance quantity
         * @property {number|null} [w] ItemInstance w
         * @property {number|null} [h] ItemInstance h
         */

        /**
         * Constructs a new ItemInstance.
         * @memberof proto
         * @classdesc Represents an ItemInstance.
         * @implements IItemInstance
         * @constructor
         * @param {proto.IItemInstance=} [properties] Properties to set
         */
        function ItemInstance(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * ItemInstance itemId.
         * @member {number|Long} itemId
         * @memberof proto.ItemInstance
         * @instance
         */
        ItemInstance.prototype.itemId = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * ItemInstance typeId.
         * @member {number} typeId
         * @memberof proto.ItemInstance
         * @instance
         */
        ItemInstance.prototype.typeId = 0;

        /**
         * ItemInstance resource.
         * @member {string} resource
         * @memberof proto.ItemInstance
         * @instance
         */
        ItemInstance.prototype.resource = "";

        /**
         * ItemInstance quality.
         * @member {number} quality
         * @memberof proto.ItemInstance
         * @instance
         */
        ItemInstance.prototype.quality = 0;

        /**
         * ItemInstance quantity.
         * @member {number} quantity
         * @memberof proto.ItemInstance
         * @instance
         */
        ItemInstance.prototype.quantity = 0;

        /**
         * ItemInstance w.
         * @member {number} w
         * @memberof proto.ItemInstance
         * @instance
         */
        ItemInstance.prototype.w = 0;

        /**
         * ItemInstance h.
         * @member {number} h
         * @memberof proto.ItemInstance
         * @instance
         */
        ItemInstance.prototype.h = 0;

        /**
         * Creates a new ItemInstance instance using the specified properties.
         * @function create
         * @memberof proto.ItemInstance
         * @static
         * @param {proto.IItemInstance=} [properties] Properties to set
         * @returns {proto.ItemInstance} ItemInstance instance
         */
        ItemInstance.create = function create(properties) {
            return new ItemInstance(properties);
        };

        /**
         * Encodes the specified ItemInstance message. Does not implicitly {@link proto.ItemInstance.verify|verify} messages.
         * @function encode
         * @memberof proto.ItemInstance
         * @static
         * @param {proto.IItemInstance} message ItemInstance message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        ItemInstance.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.itemId != null && Object.hasOwnProperty.call(message, "itemId"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint64(message.itemId);
            if (message.typeId != null && Object.hasOwnProperty.call(message, "typeId"))
                writer.uint32(/* id 2, wireType 0 =*/16).uint32(message.typeId);
            if (message.resource != null && Object.hasOwnProperty.call(message, "resource"))
                writer.uint32(/* id 3, wireType 2 =*/26).string(message.resource);
            if (message.quality != null && Object.hasOwnProperty.call(message, "quality"))
                writer.uint32(/* id 4, wireType 0 =*/32).uint32(message.quality);
            if (message.quantity != null && Object.hasOwnProperty.call(message, "quantity"))
                writer.uint32(/* id 5, wireType 0 =*/40).uint32(message.quantity);
            if (message.w != null && Object.hasOwnProperty.call(message, "w"))
                writer.uint32(/* id 10, wireType 0 =*/80).uint32(message.w);
            if (message.h != null && Object.hasOwnProperty.call(message, "h"))
                writer.uint32(/* id 11, wireType 0 =*/88).uint32(message.h);
            return writer;
        };

        /**
         * Encodes the specified ItemInstance message, length delimited. Does not implicitly {@link proto.ItemInstance.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.ItemInstance
         * @static
         * @param {proto.IItemInstance} message ItemInstance message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        ItemInstance.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes an ItemInstance message from the specified reader or buffer.
         * @function decode
         * @memberof proto.ItemInstance
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.ItemInstance} ItemInstance
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        ItemInstance.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.ItemInstance();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.itemId = reader.uint64();
                        break;
                    }
                case 2: {
                        message.typeId = reader.uint32();
                        break;
                    }
                case 3: {
                        message.resource = reader.string();
                        break;
                    }
                case 4: {
                        message.quality = reader.uint32();
                        break;
                    }
                case 5: {
                        message.quantity = reader.uint32();
                        break;
                    }
                case 10: {
                        message.w = reader.uint32();
                        break;
                    }
                case 11: {
                        message.h = reader.uint32();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes an ItemInstance message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.ItemInstance
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.ItemInstance} ItemInstance
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        ItemInstance.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies an ItemInstance message.
         * @function verify
         * @memberof proto.ItemInstance
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        ItemInstance.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.itemId != null && message.hasOwnProperty("itemId"))
                if (!$util.isInteger(message.itemId) && !(message.itemId && $util.isInteger(message.itemId.low) && $util.isInteger(message.itemId.high)))
                    return "itemId: integer|Long expected";
            if (message.typeId != null && message.hasOwnProperty("typeId"))
                if (!$util.isInteger(message.typeId))
                    return "typeId: integer expected";
            if (message.resource != null && message.hasOwnProperty("resource"))
                if (!$util.isString(message.resource))
                    return "resource: string expected";
            if (message.quality != null && message.hasOwnProperty("quality"))
                if (!$util.isInteger(message.quality))
                    return "quality: integer expected";
            if (message.quantity != null && message.hasOwnProperty("quantity"))
                if (!$util.isInteger(message.quantity))
                    return "quantity: integer expected";
            if (message.w != null && message.hasOwnProperty("w"))
                if (!$util.isInteger(message.w))
                    return "w: integer expected";
            if (message.h != null && message.hasOwnProperty("h"))
                if (!$util.isInteger(message.h))
                    return "h: integer expected";
            return null;
        };

        /**
         * Creates an ItemInstance message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.ItemInstance
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.ItemInstance} ItemInstance
         */
        ItemInstance.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.ItemInstance)
                return object;
            let message = new $root.proto.ItemInstance();
            if (object.itemId != null)
                if ($util.Long)
                    (message.itemId = $util.Long.fromValue(object.itemId)).unsigned = true;
                else if (typeof object.itemId === "string")
                    message.itemId = parseInt(object.itemId, 10);
                else if (typeof object.itemId === "number")
                    message.itemId = object.itemId;
                else if (typeof object.itemId === "object")
                    message.itemId = new $util.LongBits(object.itemId.low >>> 0, object.itemId.high >>> 0).toNumber(true);
            if (object.typeId != null)
                message.typeId = object.typeId >>> 0;
            if (object.resource != null)
                message.resource = String(object.resource);
            if (object.quality != null)
                message.quality = object.quality >>> 0;
            if (object.quantity != null)
                message.quantity = object.quantity >>> 0;
            if (object.w != null)
                message.w = object.w >>> 0;
            if (object.h != null)
                message.h = object.h >>> 0;
            return message;
        };

        /**
         * Creates a plain object from an ItemInstance message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.ItemInstance
         * @static
         * @param {proto.ItemInstance} message ItemInstance
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        ItemInstance.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                if ($util.Long) {
                    let long = new $util.Long(0, 0, true);
                    object.itemId = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.itemId = options.longs === String ? "0" : 0;
                object.typeId = 0;
                object.resource = "";
                object.quality = 0;
                object.quantity = 0;
                object.w = 0;
                object.h = 0;
            }
            if (message.itemId != null && message.hasOwnProperty("itemId"))
                if (typeof message.itemId === "number")
                    object.itemId = options.longs === String ? String(message.itemId) : message.itemId;
                else
                    object.itemId = options.longs === String ? $util.Long.prototype.toString.call(message.itemId) : options.longs === Number ? new $util.LongBits(message.itemId.low >>> 0, message.itemId.high >>> 0).toNumber(true) : message.itemId;
            if (message.typeId != null && message.hasOwnProperty("typeId"))
                object.typeId = message.typeId;
            if (message.resource != null && message.hasOwnProperty("resource"))
                object.resource = message.resource;
            if (message.quality != null && message.hasOwnProperty("quality"))
                object.quality = message.quality;
            if (message.quantity != null && message.hasOwnProperty("quantity"))
                object.quantity = message.quantity;
            if (message.w != null && message.hasOwnProperty("w"))
                object.w = message.w;
            if (message.h != null && message.hasOwnProperty("h"))
                object.h = message.h;
            return object;
        };

        /**
         * Converts this ItemInstance to JSON.
         * @function toJSON
         * @memberof proto.ItemInstance
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        ItemInstance.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for ItemInstance
         * @function getTypeUrl
         * @memberof proto.ItemInstance
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        ItemInstance.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.ItemInstance";
        };

        return ItemInstance;
    })();

    proto.GridItem = (function() {

        /**
         * Properties of a GridItem.
         * @memberof proto
         * @interface IGridItem
         * @property {number|null} [x] GridItem x
         * @property {number|null} [y] GridItem y
         * @property {proto.IItemInstance|null} [item] GridItem item
         */

        /**
         * Constructs a new GridItem.
         * @memberof proto
         * @classdesc Represents a GridItem.
         * @implements IGridItem
         * @constructor
         * @param {proto.IGridItem=} [properties] Properties to set
         */
        function GridItem(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * GridItem x.
         * @member {number} x
         * @memberof proto.GridItem
         * @instance
         */
        GridItem.prototype.x = 0;

        /**
         * GridItem y.
         * @member {number} y
         * @memberof proto.GridItem
         * @instance
         */
        GridItem.prototype.y = 0;

        /**
         * GridItem item.
         * @member {proto.IItemInstance|null|undefined} item
         * @memberof proto.GridItem
         * @instance
         */
        GridItem.prototype.item = null;

        /**
         * Creates a new GridItem instance using the specified properties.
         * @function create
         * @memberof proto.GridItem
         * @static
         * @param {proto.IGridItem=} [properties] Properties to set
         * @returns {proto.GridItem} GridItem instance
         */
        GridItem.create = function create(properties) {
            return new GridItem(properties);
        };

        /**
         * Encodes the specified GridItem message. Does not implicitly {@link proto.GridItem.verify|verify} messages.
         * @function encode
         * @memberof proto.GridItem
         * @static
         * @param {proto.IGridItem} message GridItem message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        GridItem.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.x != null && Object.hasOwnProperty.call(message, "x"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint32(message.x);
            if (message.y != null && Object.hasOwnProperty.call(message, "y"))
                writer.uint32(/* id 2, wireType 0 =*/16).uint32(message.y);
            if (message.item != null && Object.hasOwnProperty.call(message, "item"))
                $root.proto.ItemInstance.encode(message.item, writer.uint32(/* id 3, wireType 2 =*/26).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified GridItem message, length delimited. Does not implicitly {@link proto.GridItem.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.GridItem
         * @static
         * @param {proto.IGridItem} message GridItem message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        GridItem.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a GridItem message from the specified reader or buffer.
         * @function decode
         * @memberof proto.GridItem
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.GridItem} GridItem
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        GridItem.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.GridItem();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.x = reader.uint32();
                        break;
                    }
                case 2: {
                        message.y = reader.uint32();
                        break;
                    }
                case 3: {
                        message.item = $root.proto.ItemInstance.decode(reader, reader.uint32());
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a GridItem message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.GridItem
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.GridItem} GridItem
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        GridItem.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a GridItem message.
         * @function verify
         * @memberof proto.GridItem
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        GridItem.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.x != null && message.hasOwnProperty("x"))
                if (!$util.isInteger(message.x))
                    return "x: integer expected";
            if (message.y != null && message.hasOwnProperty("y"))
                if (!$util.isInteger(message.y))
                    return "y: integer expected";
            if (message.item != null && message.hasOwnProperty("item")) {
                let error = $root.proto.ItemInstance.verify(message.item);
                if (error)
                    return "item." + error;
            }
            return null;
        };

        /**
         * Creates a GridItem message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.GridItem
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.GridItem} GridItem
         */
        GridItem.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.GridItem)
                return object;
            let message = new $root.proto.GridItem();
            if (object.x != null)
                message.x = object.x >>> 0;
            if (object.y != null)
                message.y = object.y >>> 0;
            if (object.item != null) {
                if (typeof object.item !== "object")
                    throw TypeError(".proto.GridItem.item: object expected");
                message.item = $root.proto.ItemInstance.fromObject(object.item);
            }
            return message;
        };

        /**
         * Creates a plain object from a GridItem message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.GridItem
         * @static
         * @param {proto.GridItem} message GridItem
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        GridItem.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.x = 0;
                object.y = 0;
                object.item = null;
            }
            if (message.x != null && message.hasOwnProperty("x"))
                object.x = message.x;
            if (message.y != null && message.hasOwnProperty("y"))
                object.y = message.y;
            if (message.item != null && message.hasOwnProperty("item"))
                object.item = $root.proto.ItemInstance.toObject(message.item, options);
            return object;
        };

        /**
         * Converts this GridItem to JSON.
         * @function toJSON
         * @memberof proto.GridItem
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        GridItem.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for GridItem
         * @function getTypeUrl
         * @memberof proto.GridItem
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        GridItem.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.GridItem";
        };

        return GridItem;
    })();

    proto.InventoryGridState = (function() {

        /**
         * Properties of an InventoryGridState.
         * @memberof proto
         * @interface IInventoryGridState
         * @property {number|null} [width] InventoryGridState width
         * @property {number|null} [height] InventoryGridState height
         * @property {Array.<proto.IGridItem>|null} [items] InventoryGridState items
         */

        /**
         * Constructs a new InventoryGridState.
         * @memberof proto
         * @classdesc Represents an InventoryGridState.
         * @implements IInventoryGridState
         * @constructor
         * @param {proto.IInventoryGridState=} [properties] Properties to set
         */
        function InventoryGridState(properties) {
            this.items = [];
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * InventoryGridState width.
         * @member {number} width
         * @memberof proto.InventoryGridState
         * @instance
         */
        InventoryGridState.prototype.width = 0;

        /**
         * InventoryGridState height.
         * @member {number} height
         * @memberof proto.InventoryGridState
         * @instance
         */
        InventoryGridState.prototype.height = 0;

        /**
         * InventoryGridState items.
         * @member {Array.<proto.IGridItem>} items
         * @memberof proto.InventoryGridState
         * @instance
         */
        InventoryGridState.prototype.items = $util.emptyArray;

        /**
         * Creates a new InventoryGridState instance using the specified properties.
         * @function create
         * @memberof proto.InventoryGridState
         * @static
         * @param {proto.IInventoryGridState=} [properties] Properties to set
         * @returns {proto.InventoryGridState} InventoryGridState instance
         */
        InventoryGridState.create = function create(properties) {
            return new InventoryGridState(properties);
        };

        /**
         * Encodes the specified InventoryGridState message. Does not implicitly {@link proto.InventoryGridState.verify|verify} messages.
         * @function encode
         * @memberof proto.InventoryGridState
         * @static
         * @param {proto.IInventoryGridState} message InventoryGridState message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        InventoryGridState.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.width != null && Object.hasOwnProperty.call(message, "width"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint32(message.width);
            if (message.height != null && Object.hasOwnProperty.call(message, "height"))
                writer.uint32(/* id 2, wireType 0 =*/16).uint32(message.height);
            if (message.items != null && message.items.length)
                for (let i = 0; i < message.items.length; ++i)
                    $root.proto.GridItem.encode(message.items[i], writer.uint32(/* id 3, wireType 2 =*/26).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified InventoryGridState message, length delimited. Does not implicitly {@link proto.InventoryGridState.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.InventoryGridState
         * @static
         * @param {proto.IInventoryGridState} message InventoryGridState message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        InventoryGridState.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes an InventoryGridState message from the specified reader or buffer.
         * @function decode
         * @memberof proto.InventoryGridState
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.InventoryGridState} InventoryGridState
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        InventoryGridState.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.InventoryGridState();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.width = reader.uint32();
                        break;
                    }
                case 2: {
                        message.height = reader.uint32();
                        break;
                    }
                case 3: {
                        if (!(message.items && message.items.length))
                            message.items = [];
                        message.items.push($root.proto.GridItem.decode(reader, reader.uint32()));
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes an InventoryGridState message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.InventoryGridState
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.InventoryGridState} InventoryGridState
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        InventoryGridState.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies an InventoryGridState message.
         * @function verify
         * @memberof proto.InventoryGridState
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        InventoryGridState.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.width != null && message.hasOwnProperty("width"))
                if (!$util.isInteger(message.width))
                    return "width: integer expected";
            if (message.height != null && message.hasOwnProperty("height"))
                if (!$util.isInteger(message.height))
                    return "height: integer expected";
            if (message.items != null && message.hasOwnProperty("items")) {
                if (!Array.isArray(message.items))
                    return "items: array expected";
                for (let i = 0; i < message.items.length; ++i) {
                    let error = $root.proto.GridItem.verify(message.items[i]);
                    if (error)
                        return "items." + error;
                }
            }
            return null;
        };

        /**
         * Creates an InventoryGridState message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.InventoryGridState
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.InventoryGridState} InventoryGridState
         */
        InventoryGridState.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.InventoryGridState)
                return object;
            let message = new $root.proto.InventoryGridState();
            if (object.width != null)
                message.width = object.width >>> 0;
            if (object.height != null)
                message.height = object.height >>> 0;
            if (object.items) {
                if (!Array.isArray(object.items))
                    throw TypeError(".proto.InventoryGridState.items: array expected");
                message.items = [];
                for (let i = 0; i < object.items.length; ++i) {
                    if (typeof object.items[i] !== "object")
                        throw TypeError(".proto.InventoryGridState.items: object expected");
                    message.items[i] = $root.proto.GridItem.fromObject(object.items[i]);
                }
            }
            return message;
        };

        /**
         * Creates a plain object from an InventoryGridState message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.InventoryGridState
         * @static
         * @param {proto.InventoryGridState} message InventoryGridState
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        InventoryGridState.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.arrays || options.defaults)
                object.items = [];
            if (options.defaults) {
                object.width = 0;
                object.height = 0;
            }
            if (message.width != null && message.hasOwnProperty("width"))
                object.width = message.width;
            if (message.height != null && message.hasOwnProperty("height"))
                object.height = message.height;
            if (message.items && message.items.length) {
                object.items = [];
                for (let j = 0; j < message.items.length; ++j)
                    object.items[j] = $root.proto.GridItem.toObject(message.items[j], options);
            }
            return object;
        };

        /**
         * Converts this InventoryGridState to JSON.
         * @function toJSON
         * @memberof proto.InventoryGridState
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        InventoryGridState.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for InventoryGridState
         * @function getTypeUrl
         * @memberof proto.InventoryGridState
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        InventoryGridState.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.InventoryGridState";
        };

        return InventoryGridState;
    })();

    proto.EquipmentItem = (function() {

        /**
         * Properties of an EquipmentItem.
         * @memberof proto
         * @interface IEquipmentItem
         * @property {proto.EquipSlot|null} [slot] EquipmentItem slot
         * @property {proto.IItemInstance|null} [item] EquipmentItem item
         */

        /**
         * Constructs a new EquipmentItem.
         * @memberof proto
         * @classdesc Represents an EquipmentItem.
         * @implements IEquipmentItem
         * @constructor
         * @param {proto.IEquipmentItem=} [properties] Properties to set
         */
        function EquipmentItem(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * EquipmentItem slot.
         * @member {proto.EquipSlot} slot
         * @memberof proto.EquipmentItem
         * @instance
         */
        EquipmentItem.prototype.slot = 0;

        /**
         * EquipmentItem item.
         * @member {proto.IItemInstance|null|undefined} item
         * @memberof proto.EquipmentItem
         * @instance
         */
        EquipmentItem.prototype.item = null;

        /**
         * Creates a new EquipmentItem instance using the specified properties.
         * @function create
         * @memberof proto.EquipmentItem
         * @static
         * @param {proto.IEquipmentItem=} [properties] Properties to set
         * @returns {proto.EquipmentItem} EquipmentItem instance
         */
        EquipmentItem.create = function create(properties) {
            return new EquipmentItem(properties);
        };

        /**
         * Encodes the specified EquipmentItem message. Does not implicitly {@link proto.EquipmentItem.verify|verify} messages.
         * @function encode
         * @memberof proto.EquipmentItem
         * @static
         * @param {proto.IEquipmentItem} message EquipmentItem message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        EquipmentItem.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.slot != null && Object.hasOwnProperty.call(message, "slot"))
                writer.uint32(/* id 1, wireType 0 =*/8).int32(message.slot);
            if (message.item != null && Object.hasOwnProperty.call(message, "item"))
                $root.proto.ItemInstance.encode(message.item, writer.uint32(/* id 2, wireType 2 =*/18).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified EquipmentItem message, length delimited. Does not implicitly {@link proto.EquipmentItem.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.EquipmentItem
         * @static
         * @param {proto.IEquipmentItem} message EquipmentItem message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        EquipmentItem.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes an EquipmentItem message from the specified reader or buffer.
         * @function decode
         * @memberof proto.EquipmentItem
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.EquipmentItem} EquipmentItem
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        EquipmentItem.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.EquipmentItem();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.slot = reader.int32();
                        break;
                    }
                case 2: {
                        message.item = $root.proto.ItemInstance.decode(reader, reader.uint32());
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes an EquipmentItem message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.EquipmentItem
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.EquipmentItem} EquipmentItem
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        EquipmentItem.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies an EquipmentItem message.
         * @function verify
         * @memberof proto.EquipmentItem
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        EquipmentItem.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.slot != null && message.hasOwnProperty("slot"))
                switch (message.slot) {
                default:
                    return "slot: enum value expected";
                case 0:
                case 1:
                case 2:
                case 3:
                case 4:
                case 5:
                case 6:
                case 7:
                case 8:
                case 9:
                case 10:
                case 11:
                    break;
                }
            if (message.item != null && message.hasOwnProperty("item")) {
                let error = $root.proto.ItemInstance.verify(message.item);
                if (error)
                    return "item." + error;
            }
            return null;
        };

        /**
         * Creates an EquipmentItem message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.EquipmentItem
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.EquipmentItem} EquipmentItem
         */
        EquipmentItem.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.EquipmentItem)
                return object;
            let message = new $root.proto.EquipmentItem();
            switch (object.slot) {
            default:
                if (typeof object.slot === "number") {
                    message.slot = object.slot;
                    break;
                }
                break;
            case "EQUIP_SLOT_NONE":
            case 0:
                message.slot = 0;
                break;
            case "EQUIP_SLOT_HEAD":
            case 1:
                message.slot = 1;
                break;
            case "EQUIP_SLOT_CHEST":
            case 2:
                message.slot = 2;
                break;
            case "EQUIP_SLOT_LEGS":
            case 3:
                message.slot = 3;
                break;
            case "EQUIP_SLOT_FEET":
            case 4:
                message.slot = 4;
                break;
            case "EQUIP_SLOT_HANDS":
            case 5:
                message.slot = 5;
                break;
            case "EQUIP_SLOT_LEFT_HAND":
            case 6:
                message.slot = 6;
                break;
            case "EQUIP_SLOT_RIGHT_HAND":
            case 7:
                message.slot = 7;
                break;
            case "EQUIP_SLOT_BACK":
            case 8:
                message.slot = 8;
                break;
            case "EQUIP_SLOT_NECK":
            case 9:
                message.slot = 9;
                break;
            case "EQUIP_SLOT_RING_1":
            case 10:
                message.slot = 10;
                break;
            case "EQUIP_SLOT_RING_2":
            case 11:
                message.slot = 11;
                break;
            }
            if (object.item != null) {
                if (typeof object.item !== "object")
                    throw TypeError(".proto.EquipmentItem.item: object expected");
                message.item = $root.proto.ItemInstance.fromObject(object.item);
            }
            return message;
        };

        /**
         * Creates a plain object from an EquipmentItem message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.EquipmentItem
         * @static
         * @param {proto.EquipmentItem} message EquipmentItem
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        EquipmentItem.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.slot = options.enums === String ? "EQUIP_SLOT_NONE" : 0;
                object.item = null;
            }
            if (message.slot != null && message.hasOwnProperty("slot"))
                object.slot = options.enums === String ? $root.proto.EquipSlot[message.slot] === undefined ? message.slot : $root.proto.EquipSlot[message.slot] : message.slot;
            if (message.item != null && message.hasOwnProperty("item"))
                object.item = $root.proto.ItemInstance.toObject(message.item, options);
            return object;
        };

        /**
         * Converts this EquipmentItem to JSON.
         * @function toJSON
         * @memberof proto.EquipmentItem
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        EquipmentItem.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for EquipmentItem
         * @function getTypeUrl
         * @memberof proto.EquipmentItem
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        EquipmentItem.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.EquipmentItem";
        };

        return EquipmentItem;
    })();

    proto.InventoryEquipmentState = (function() {

        /**
         * Properties of an InventoryEquipmentState.
         * @memberof proto
         * @interface IInventoryEquipmentState
         * @property {Array.<proto.IEquipmentItem>|null} [items] InventoryEquipmentState items
         */

        /**
         * Constructs a new InventoryEquipmentState.
         * @memberof proto
         * @classdesc Represents an InventoryEquipmentState.
         * @implements IInventoryEquipmentState
         * @constructor
         * @param {proto.IInventoryEquipmentState=} [properties] Properties to set
         */
        function InventoryEquipmentState(properties) {
            this.items = [];
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * InventoryEquipmentState items.
         * @member {Array.<proto.IEquipmentItem>} items
         * @memberof proto.InventoryEquipmentState
         * @instance
         */
        InventoryEquipmentState.prototype.items = $util.emptyArray;

        /**
         * Creates a new InventoryEquipmentState instance using the specified properties.
         * @function create
         * @memberof proto.InventoryEquipmentState
         * @static
         * @param {proto.IInventoryEquipmentState=} [properties] Properties to set
         * @returns {proto.InventoryEquipmentState} InventoryEquipmentState instance
         */
        InventoryEquipmentState.create = function create(properties) {
            return new InventoryEquipmentState(properties);
        };

        /**
         * Encodes the specified InventoryEquipmentState message. Does not implicitly {@link proto.InventoryEquipmentState.verify|verify} messages.
         * @function encode
         * @memberof proto.InventoryEquipmentState
         * @static
         * @param {proto.IInventoryEquipmentState} message InventoryEquipmentState message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        InventoryEquipmentState.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.items != null && message.items.length)
                for (let i = 0; i < message.items.length; ++i)
                    $root.proto.EquipmentItem.encode(message.items[i], writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified InventoryEquipmentState message, length delimited. Does not implicitly {@link proto.InventoryEquipmentState.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.InventoryEquipmentState
         * @static
         * @param {proto.IInventoryEquipmentState} message InventoryEquipmentState message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        InventoryEquipmentState.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes an InventoryEquipmentState message from the specified reader or buffer.
         * @function decode
         * @memberof proto.InventoryEquipmentState
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.InventoryEquipmentState} InventoryEquipmentState
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        InventoryEquipmentState.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.InventoryEquipmentState();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        if (!(message.items && message.items.length))
                            message.items = [];
                        message.items.push($root.proto.EquipmentItem.decode(reader, reader.uint32()));
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes an InventoryEquipmentState message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.InventoryEquipmentState
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.InventoryEquipmentState} InventoryEquipmentState
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        InventoryEquipmentState.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies an InventoryEquipmentState message.
         * @function verify
         * @memberof proto.InventoryEquipmentState
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        InventoryEquipmentState.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.items != null && message.hasOwnProperty("items")) {
                if (!Array.isArray(message.items))
                    return "items: array expected";
                for (let i = 0; i < message.items.length; ++i) {
                    let error = $root.proto.EquipmentItem.verify(message.items[i]);
                    if (error)
                        return "items." + error;
                }
            }
            return null;
        };

        /**
         * Creates an InventoryEquipmentState message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.InventoryEquipmentState
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.InventoryEquipmentState} InventoryEquipmentState
         */
        InventoryEquipmentState.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.InventoryEquipmentState)
                return object;
            let message = new $root.proto.InventoryEquipmentState();
            if (object.items) {
                if (!Array.isArray(object.items))
                    throw TypeError(".proto.InventoryEquipmentState.items: array expected");
                message.items = [];
                for (let i = 0; i < object.items.length; ++i) {
                    if (typeof object.items[i] !== "object")
                        throw TypeError(".proto.InventoryEquipmentState.items: object expected");
                    message.items[i] = $root.proto.EquipmentItem.fromObject(object.items[i]);
                }
            }
            return message;
        };

        /**
         * Creates a plain object from an InventoryEquipmentState message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.InventoryEquipmentState
         * @static
         * @param {proto.InventoryEquipmentState} message InventoryEquipmentState
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        InventoryEquipmentState.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.arrays || options.defaults)
                object.items = [];
            if (message.items && message.items.length) {
                object.items = [];
                for (let j = 0; j < message.items.length; ++j)
                    object.items[j] = $root.proto.EquipmentItem.toObject(message.items[j], options);
            }
            return object;
        };

        /**
         * Converts this InventoryEquipmentState to JSON.
         * @function toJSON
         * @memberof proto.InventoryEquipmentState
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        InventoryEquipmentState.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for InventoryEquipmentState
         * @function getTypeUrl
         * @memberof proto.InventoryEquipmentState
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        InventoryEquipmentState.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.InventoryEquipmentState";
        };

        return InventoryEquipmentState;
    })();

    proto.InventoryHandState = (function() {

        /**
         * Properties of an InventoryHandState.
         * @memberof proto
         * @interface IInventoryHandState
         * @property {proto.IItemInstance|null} [item] InventoryHandState item
         */

        /**
         * Constructs a new InventoryHandState.
         * @memberof proto
         * @classdesc Represents an InventoryHandState.
         * @implements IInventoryHandState
         * @constructor
         * @param {proto.IInventoryHandState=} [properties] Properties to set
         */
        function InventoryHandState(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * InventoryHandState item.
         * @member {proto.IItemInstance|null|undefined} item
         * @memberof proto.InventoryHandState
         * @instance
         */
        InventoryHandState.prototype.item = null;

        // OneOf field names bound to virtual getters and setters
        let $oneOfFields;

        // Virtual OneOf for proto3 optional field
        Object.defineProperty(InventoryHandState.prototype, "_item", {
            get: $util.oneOfGetter($oneOfFields = ["item"]),
            set: $util.oneOfSetter($oneOfFields)
        });

        /**
         * Creates a new InventoryHandState instance using the specified properties.
         * @function create
         * @memberof proto.InventoryHandState
         * @static
         * @param {proto.IInventoryHandState=} [properties] Properties to set
         * @returns {proto.InventoryHandState} InventoryHandState instance
         */
        InventoryHandState.create = function create(properties) {
            return new InventoryHandState(properties);
        };

        /**
         * Encodes the specified InventoryHandState message. Does not implicitly {@link proto.InventoryHandState.verify|verify} messages.
         * @function encode
         * @memberof proto.InventoryHandState
         * @static
         * @param {proto.IInventoryHandState} message InventoryHandState message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        InventoryHandState.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.item != null && Object.hasOwnProperty.call(message, "item"))
                $root.proto.ItemInstance.encode(message.item, writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified InventoryHandState message, length delimited. Does not implicitly {@link proto.InventoryHandState.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.InventoryHandState
         * @static
         * @param {proto.IInventoryHandState} message InventoryHandState message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        InventoryHandState.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes an InventoryHandState message from the specified reader or buffer.
         * @function decode
         * @memberof proto.InventoryHandState
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.InventoryHandState} InventoryHandState
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        InventoryHandState.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.InventoryHandState();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.item = $root.proto.ItemInstance.decode(reader, reader.uint32());
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes an InventoryHandState message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.InventoryHandState
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.InventoryHandState} InventoryHandState
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        InventoryHandState.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies an InventoryHandState message.
         * @function verify
         * @memberof proto.InventoryHandState
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        InventoryHandState.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            let properties = {};
            if (message.item != null && message.hasOwnProperty("item")) {
                properties._item = 1;
                {
                    let error = $root.proto.ItemInstance.verify(message.item);
                    if (error)
                        return "item." + error;
                }
            }
            return null;
        };

        /**
         * Creates an InventoryHandState message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.InventoryHandState
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.InventoryHandState} InventoryHandState
         */
        InventoryHandState.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.InventoryHandState)
                return object;
            let message = new $root.proto.InventoryHandState();
            if (object.item != null) {
                if (typeof object.item !== "object")
                    throw TypeError(".proto.InventoryHandState.item: object expected");
                message.item = $root.proto.ItemInstance.fromObject(object.item);
            }
            return message;
        };

        /**
         * Creates a plain object from an InventoryHandState message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.InventoryHandState
         * @static
         * @param {proto.InventoryHandState} message InventoryHandState
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        InventoryHandState.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (message.item != null && message.hasOwnProperty("item")) {
                object.item = $root.proto.ItemInstance.toObject(message.item, options);
                if (options.oneofs)
                    object._item = "item";
            }
            return object;
        };

        /**
         * Converts this InventoryHandState to JSON.
         * @function toJSON
         * @memberof proto.InventoryHandState
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        InventoryHandState.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for InventoryHandState
         * @function getTypeUrl
         * @memberof proto.InventoryHandState
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        InventoryHandState.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.InventoryHandState";
        };

        return InventoryHandState;
    })();

    proto.InventoryState = (function() {

        /**
         * Properties of an InventoryState.
         * @memberof proto
         * @interface IInventoryState
         * @property {proto.IInventoryRef|null} [ref] InventoryState ref
         * @property {number|Long|null} [revision] InventoryState revision
         * @property {proto.IInventoryGridState|null} [grid] InventoryState grid
         * @property {proto.IInventoryEquipmentState|null} [equipment] InventoryState equipment
         * @property {proto.IInventoryHandState|null} [hand] InventoryState hand
         */

        /**
         * Constructs a new InventoryState.
         * @memberof proto
         * @classdesc Represents an InventoryState.
         * @implements IInventoryState
         * @constructor
         * @param {proto.IInventoryState=} [properties] Properties to set
         */
        function InventoryState(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * InventoryState ref.
         * @member {proto.IInventoryRef|null|undefined} ref
         * @memberof proto.InventoryState
         * @instance
         */
        InventoryState.prototype.ref = null;

        /**
         * InventoryState revision.
         * @member {number|Long} revision
         * @memberof proto.InventoryState
         * @instance
         */
        InventoryState.prototype.revision = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * InventoryState grid.
         * @member {proto.IInventoryGridState|null|undefined} grid
         * @memberof proto.InventoryState
         * @instance
         */
        InventoryState.prototype.grid = null;

        /**
         * InventoryState equipment.
         * @member {proto.IInventoryEquipmentState|null|undefined} equipment
         * @memberof proto.InventoryState
         * @instance
         */
        InventoryState.prototype.equipment = null;

        /**
         * InventoryState hand.
         * @member {proto.IInventoryHandState|null|undefined} hand
         * @memberof proto.InventoryState
         * @instance
         */
        InventoryState.prototype.hand = null;

        // OneOf field names bound to virtual getters and setters
        let $oneOfFields;

        /**
         * InventoryState state.
         * @member {"grid"|"equipment"|"hand"|undefined} state
         * @memberof proto.InventoryState
         * @instance
         */
        Object.defineProperty(InventoryState.prototype, "state", {
            get: $util.oneOfGetter($oneOfFields = ["grid", "equipment", "hand"]),
            set: $util.oneOfSetter($oneOfFields)
        });

        /**
         * Creates a new InventoryState instance using the specified properties.
         * @function create
         * @memberof proto.InventoryState
         * @static
         * @param {proto.IInventoryState=} [properties] Properties to set
         * @returns {proto.InventoryState} InventoryState instance
         */
        InventoryState.create = function create(properties) {
            return new InventoryState(properties);
        };

        /**
         * Encodes the specified InventoryState message. Does not implicitly {@link proto.InventoryState.verify|verify} messages.
         * @function encode
         * @memberof proto.InventoryState
         * @static
         * @param {proto.IInventoryState} message InventoryState message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        InventoryState.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.ref != null && Object.hasOwnProperty.call(message, "ref"))
                $root.proto.InventoryRef.encode(message.ref, writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
            if (message.revision != null && Object.hasOwnProperty.call(message, "revision"))
                writer.uint32(/* id 2, wireType 0 =*/16).uint64(message.revision);
            if (message.grid != null && Object.hasOwnProperty.call(message, "grid"))
                $root.proto.InventoryGridState.encode(message.grid, writer.uint32(/* id 10, wireType 2 =*/82).fork()).ldelim();
            if (message.equipment != null && Object.hasOwnProperty.call(message, "equipment"))
                $root.proto.InventoryEquipmentState.encode(message.equipment, writer.uint32(/* id 11, wireType 2 =*/90).fork()).ldelim();
            if (message.hand != null && Object.hasOwnProperty.call(message, "hand"))
                $root.proto.InventoryHandState.encode(message.hand, writer.uint32(/* id 12, wireType 2 =*/98).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified InventoryState message, length delimited. Does not implicitly {@link proto.InventoryState.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.InventoryState
         * @static
         * @param {proto.IInventoryState} message InventoryState message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        InventoryState.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes an InventoryState message from the specified reader or buffer.
         * @function decode
         * @memberof proto.InventoryState
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.InventoryState} InventoryState
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        InventoryState.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.InventoryState();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.ref = $root.proto.InventoryRef.decode(reader, reader.uint32());
                        break;
                    }
                case 2: {
                        message.revision = reader.uint64();
                        break;
                    }
                case 10: {
                        message.grid = $root.proto.InventoryGridState.decode(reader, reader.uint32());
                        break;
                    }
                case 11: {
                        message.equipment = $root.proto.InventoryEquipmentState.decode(reader, reader.uint32());
                        break;
                    }
                case 12: {
                        message.hand = $root.proto.InventoryHandState.decode(reader, reader.uint32());
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes an InventoryState message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.InventoryState
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.InventoryState} InventoryState
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        InventoryState.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies an InventoryState message.
         * @function verify
         * @memberof proto.InventoryState
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        InventoryState.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            let properties = {};
            if (message.ref != null && message.hasOwnProperty("ref")) {
                let error = $root.proto.InventoryRef.verify(message.ref);
                if (error)
                    return "ref." + error;
            }
            if (message.revision != null && message.hasOwnProperty("revision"))
                if (!$util.isInteger(message.revision) && !(message.revision && $util.isInteger(message.revision.low) && $util.isInteger(message.revision.high)))
                    return "revision: integer|Long expected";
            if (message.grid != null && message.hasOwnProperty("grid")) {
                properties.state = 1;
                {
                    let error = $root.proto.InventoryGridState.verify(message.grid);
                    if (error)
                        return "grid." + error;
                }
            }
            if (message.equipment != null && message.hasOwnProperty("equipment")) {
                if (properties.state === 1)
                    return "state: multiple values";
                properties.state = 1;
                {
                    let error = $root.proto.InventoryEquipmentState.verify(message.equipment);
                    if (error)
                        return "equipment." + error;
                }
            }
            if (message.hand != null && message.hasOwnProperty("hand")) {
                if (properties.state === 1)
                    return "state: multiple values";
                properties.state = 1;
                {
                    let error = $root.proto.InventoryHandState.verify(message.hand);
                    if (error)
                        return "hand." + error;
                }
            }
            return null;
        };

        /**
         * Creates an InventoryState message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.InventoryState
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.InventoryState} InventoryState
         */
        InventoryState.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.InventoryState)
                return object;
            let message = new $root.proto.InventoryState();
            if (object.ref != null) {
                if (typeof object.ref !== "object")
                    throw TypeError(".proto.InventoryState.ref: object expected");
                message.ref = $root.proto.InventoryRef.fromObject(object.ref);
            }
            if (object.revision != null)
                if ($util.Long)
                    (message.revision = $util.Long.fromValue(object.revision)).unsigned = true;
                else if (typeof object.revision === "string")
                    message.revision = parseInt(object.revision, 10);
                else if (typeof object.revision === "number")
                    message.revision = object.revision;
                else if (typeof object.revision === "object")
                    message.revision = new $util.LongBits(object.revision.low >>> 0, object.revision.high >>> 0).toNumber(true);
            if (object.grid != null) {
                if (typeof object.grid !== "object")
                    throw TypeError(".proto.InventoryState.grid: object expected");
                message.grid = $root.proto.InventoryGridState.fromObject(object.grid);
            }
            if (object.equipment != null) {
                if (typeof object.equipment !== "object")
                    throw TypeError(".proto.InventoryState.equipment: object expected");
                message.equipment = $root.proto.InventoryEquipmentState.fromObject(object.equipment);
            }
            if (object.hand != null) {
                if (typeof object.hand !== "object")
                    throw TypeError(".proto.InventoryState.hand: object expected");
                message.hand = $root.proto.InventoryHandState.fromObject(object.hand);
            }
            return message;
        };

        /**
         * Creates a plain object from an InventoryState message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.InventoryState
         * @static
         * @param {proto.InventoryState} message InventoryState
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        InventoryState.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.ref = null;
                if ($util.Long) {
                    let long = new $util.Long(0, 0, true);
                    object.revision = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.revision = options.longs === String ? "0" : 0;
            }
            if (message.ref != null && message.hasOwnProperty("ref"))
                object.ref = $root.proto.InventoryRef.toObject(message.ref, options);
            if (message.revision != null && message.hasOwnProperty("revision"))
                if (typeof message.revision === "number")
                    object.revision = options.longs === String ? String(message.revision) : message.revision;
                else
                    object.revision = options.longs === String ? $util.Long.prototype.toString.call(message.revision) : options.longs === Number ? new $util.LongBits(message.revision.low >>> 0, message.revision.high >>> 0).toNumber(true) : message.revision;
            if (message.grid != null && message.hasOwnProperty("grid")) {
                object.grid = $root.proto.InventoryGridState.toObject(message.grid, options);
                if (options.oneofs)
                    object.state = "grid";
            }
            if (message.equipment != null && message.hasOwnProperty("equipment")) {
                object.equipment = $root.proto.InventoryEquipmentState.toObject(message.equipment, options);
                if (options.oneofs)
                    object.state = "equipment";
            }
            if (message.hand != null && message.hasOwnProperty("hand")) {
                object.hand = $root.proto.InventoryHandState.toObject(message.hand, options);
                if (options.oneofs)
                    object.state = "hand";
            }
            return object;
        };

        /**
         * Converts this InventoryState to JSON.
         * @function toJSON
         * @memberof proto.InventoryState
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        InventoryState.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for InventoryState
         * @function getTypeUrl
         * @memberof proto.InventoryState
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        InventoryState.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.InventoryState";
        };

        return InventoryState;
    })();

    proto.InventoryExpected = (function() {

        /**
         * Properties of an InventoryExpected.
         * @memberof proto
         * @interface IInventoryExpected
         * @property {proto.IInventoryRef|null} [ref] InventoryExpected ref
         * @property {number|Long|null} [expectedRevision] InventoryExpected expectedRevision
         */

        /**
         * Constructs a new InventoryExpected.
         * @memberof proto
         * @classdesc Represents an InventoryExpected.
         * @implements IInventoryExpected
         * @constructor
         * @param {proto.IInventoryExpected=} [properties] Properties to set
         */
        function InventoryExpected(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * InventoryExpected ref.
         * @member {proto.IInventoryRef|null|undefined} ref
         * @memberof proto.InventoryExpected
         * @instance
         */
        InventoryExpected.prototype.ref = null;

        /**
         * InventoryExpected expectedRevision.
         * @member {number|Long} expectedRevision
         * @memberof proto.InventoryExpected
         * @instance
         */
        InventoryExpected.prototype.expectedRevision = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * Creates a new InventoryExpected instance using the specified properties.
         * @function create
         * @memberof proto.InventoryExpected
         * @static
         * @param {proto.IInventoryExpected=} [properties] Properties to set
         * @returns {proto.InventoryExpected} InventoryExpected instance
         */
        InventoryExpected.create = function create(properties) {
            return new InventoryExpected(properties);
        };

        /**
         * Encodes the specified InventoryExpected message. Does not implicitly {@link proto.InventoryExpected.verify|verify} messages.
         * @function encode
         * @memberof proto.InventoryExpected
         * @static
         * @param {proto.IInventoryExpected} message InventoryExpected message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        InventoryExpected.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.ref != null && Object.hasOwnProperty.call(message, "ref"))
                $root.proto.InventoryRef.encode(message.ref, writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
            if (message.expectedRevision != null && Object.hasOwnProperty.call(message, "expectedRevision"))
                writer.uint32(/* id 2, wireType 0 =*/16).uint64(message.expectedRevision);
            return writer;
        };

        /**
         * Encodes the specified InventoryExpected message, length delimited. Does not implicitly {@link proto.InventoryExpected.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.InventoryExpected
         * @static
         * @param {proto.IInventoryExpected} message InventoryExpected message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        InventoryExpected.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes an InventoryExpected message from the specified reader or buffer.
         * @function decode
         * @memberof proto.InventoryExpected
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.InventoryExpected} InventoryExpected
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        InventoryExpected.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.InventoryExpected();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.ref = $root.proto.InventoryRef.decode(reader, reader.uint32());
                        break;
                    }
                case 2: {
                        message.expectedRevision = reader.uint64();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes an InventoryExpected message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.InventoryExpected
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.InventoryExpected} InventoryExpected
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        InventoryExpected.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies an InventoryExpected message.
         * @function verify
         * @memberof proto.InventoryExpected
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        InventoryExpected.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.ref != null && message.hasOwnProperty("ref")) {
                let error = $root.proto.InventoryRef.verify(message.ref);
                if (error)
                    return "ref." + error;
            }
            if (message.expectedRevision != null && message.hasOwnProperty("expectedRevision"))
                if (!$util.isInteger(message.expectedRevision) && !(message.expectedRevision && $util.isInteger(message.expectedRevision.low) && $util.isInteger(message.expectedRevision.high)))
                    return "expectedRevision: integer|Long expected";
            return null;
        };

        /**
         * Creates an InventoryExpected message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.InventoryExpected
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.InventoryExpected} InventoryExpected
         */
        InventoryExpected.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.InventoryExpected)
                return object;
            let message = new $root.proto.InventoryExpected();
            if (object.ref != null) {
                if (typeof object.ref !== "object")
                    throw TypeError(".proto.InventoryExpected.ref: object expected");
                message.ref = $root.proto.InventoryRef.fromObject(object.ref);
            }
            if (object.expectedRevision != null)
                if ($util.Long)
                    (message.expectedRevision = $util.Long.fromValue(object.expectedRevision)).unsigned = true;
                else if (typeof object.expectedRevision === "string")
                    message.expectedRevision = parseInt(object.expectedRevision, 10);
                else if (typeof object.expectedRevision === "number")
                    message.expectedRevision = object.expectedRevision;
                else if (typeof object.expectedRevision === "object")
                    message.expectedRevision = new $util.LongBits(object.expectedRevision.low >>> 0, object.expectedRevision.high >>> 0).toNumber(true);
            return message;
        };

        /**
         * Creates a plain object from an InventoryExpected message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.InventoryExpected
         * @static
         * @param {proto.InventoryExpected} message InventoryExpected
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        InventoryExpected.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.ref = null;
                if ($util.Long) {
                    let long = new $util.Long(0, 0, true);
                    object.expectedRevision = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.expectedRevision = options.longs === String ? "0" : 0;
            }
            if (message.ref != null && message.hasOwnProperty("ref"))
                object.ref = $root.proto.InventoryRef.toObject(message.ref, options);
            if (message.expectedRevision != null && message.hasOwnProperty("expectedRevision"))
                if (typeof message.expectedRevision === "number")
                    object.expectedRevision = options.longs === String ? String(message.expectedRevision) : message.expectedRevision;
                else
                    object.expectedRevision = options.longs === String ? $util.Long.prototype.toString.call(message.expectedRevision) : options.longs === Number ? new $util.LongBits(message.expectedRevision.low >>> 0, message.expectedRevision.high >>> 0).toNumber(true) : message.expectedRevision;
            return object;
        };

        /**
         * Converts this InventoryExpected to JSON.
         * @function toJSON
         * @memberof proto.InventoryExpected
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        InventoryExpected.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for InventoryExpected
         * @function getTypeUrl
         * @memberof proto.InventoryExpected
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        InventoryExpected.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.InventoryExpected";
        };

        return InventoryExpected;
    })();

    proto.GridPos = (function() {

        /**
         * Properties of a GridPos.
         * @memberof proto
         * @interface IGridPos
         * @property {number|null} [x] GridPos x
         * @property {number|null} [y] GridPos y
         */

        /**
         * Constructs a new GridPos.
         * @memberof proto
         * @classdesc Represents a GridPos.
         * @implements IGridPos
         * @constructor
         * @param {proto.IGridPos=} [properties] Properties to set
         */
        function GridPos(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * GridPos x.
         * @member {number} x
         * @memberof proto.GridPos
         * @instance
         */
        GridPos.prototype.x = 0;

        /**
         * GridPos y.
         * @member {number} y
         * @memberof proto.GridPos
         * @instance
         */
        GridPos.prototype.y = 0;

        /**
         * Creates a new GridPos instance using the specified properties.
         * @function create
         * @memberof proto.GridPos
         * @static
         * @param {proto.IGridPos=} [properties] Properties to set
         * @returns {proto.GridPos} GridPos instance
         */
        GridPos.create = function create(properties) {
            return new GridPos(properties);
        };

        /**
         * Encodes the specified GridPos message. Does not implicitly {@link proto.GridPos.verify|verify} messages.
         * @function encode
         * @memberof proto.GridPos
         * @static
         * @param {proto.IGridPos} message GridPos message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        GridPos.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.x != null && Object.hasOwnProperty.call(message, "x"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint32(message.x);
            if (message.y != null && Object.hasOwnProperty.call(message, "y"))
                writer.uint32(/* id 2, wireType 0 =*/16).uint32(message.y);
            return writer;
        };

        /**
         * Encodes the specified GridPos message, length delimited. Does not implicitly {@link proto.GridPos.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.GridPos
         * @static
         * @param {proto.IGridPos} message GridPos message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        GridPos.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a GridPos message from the specified reader or buffer.
         * @function decode
         * @memberof proto.GridPos
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.GridPos} GridPos
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        GridPos.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.GridPos();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.x = reader.uint32();
                        break;
                    }
                case 2: {
                        message.y = reader.uint32();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a GridPos message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.GridPos
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.GridPos} GridPos
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        GridPos.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a GridPos message.
         * @function verify
         * @memberof proto.GridPos
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        GridPos.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.x != null && message.hasOwnProperty("x"))
                if (!$util.isInteger(message.x))
                    return "x: integer expected";
            if (message.y != null && message.hasOwnProperty("y"))
                if (!$util.isInteger(message.y))
                    return "y: integer expected";
            return null;
        };

        /**
         * Creates a GridPos message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.GridPos
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.GridPos} GridPos
         */
        GridPos.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.GridPos)
                return object;
            let message = new $root.proto.GridPos();
            if (object.x != null)
                message.x = object.x >>> 0;
            if (object.y != null)
                message.y = object.y >>> 0;
            return message;
        };

        /**
         * Creates a plain object from a GridPos message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.GridPos
         * @static
         * @param {proto.GridPos} message GridPos
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        GridPos.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.x = 0;
                object.y = 0;
            }
            if (message.x != null && message.hasOwnProperty("x"))
                object.x = message.x;
            if (message.y != null && message.hasOwnProperty("y"))
                object.y = message.y;
            return object;
        };

        /**
         * Converts this GridPos to JSON.
         * @function toJSON
         * @memberof proto.GridPos
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        GridPos.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for GridPos
         * @function getTypeUrl
         * @memberof proto.GridPos
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        GridPos.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.GridPos";
        };

        return GridPos;
    })();

    proto.InventoryMoveSpec = (function() {

        /**
         * Properties of an InventoryMoveSpec.
         * @memberof proto
         * @interface IInventoryMoveSpec
         * @property {proto.IInventoryRef|null} [src] InventoryMoveSpec src
         * @property {proto.IInventoryRef|null} [dst] InventoryMoveSpec dst
         * @property {number|Long|null} [itemId] InventoryMoveSpec itemId
         * @property {proto.IGridPos|null} [dstPos] InventoryMoveSpec dstPos
         * @property {proto.EquipSlot|null} [dstEquipSlot] InventoryMoveSpec dstEquipSlot
         * @property {number|null} [quantity] InventoryMoveSpec quantity
         * @property {boolean|null} [allowSwapOrMerge] InventoryMoveSpec allowSwapOrMerge
         */

        /**
         * Constructs a new InventoryMoveSpec.
         * @memberof proto
         * @classdesc Represents an InventoryMoveSpec.
         * @implements IInventoryMoveSpec
         * @constructor
         * @param {proto.IInventoryMoveSpec=} [properties] Properties to set
         */
        function InventoryMoveSpec(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * InventoryMoveSpec src.
         * @member {proto.IInventoryRef|null|undefined} src
         * @memberof proto.InventoryMoveSpec
         * @instance
         */
        InventoryMoveSpec.prototype.src = null;

        /**
         * InventoryMoveSpec dst.
         * @member {proto.IInventoryRef|null|undefined} dst
         * @memberof proto.InventoryMoveSpec
         * @instance
         */
        InventoryMoveSpec.prototype.dst = null;

        /**
         * InventoryMoveSpec itemId.
         * @member {number|Long} itemId
         * @memberof proto.InventoryMoveSpec
         * @instance
         */
        InventoryMoveSpec.prototype.itemId = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * InventoryMoveSpec dstPos.
         * @member {proto.IGridPos|null|undefined} dstPos
         * @memberof proto.InventoryMoveSpec
         * @instance
         */
        InventoryMoveSpec.prototype.dstPos = null;

        /**
         * InventoryMoveSpec dstEquipSlot.
         * @member {proto.EquipSlot|null|undefined} dstEquipSlot
         * @memberof proto.InventoryMoveSpec
         * @instance
         */
        InventoryMoveSpec.prototype.dstEquipSlot = null;

        /**
         * InventoryMoveSpec quantity.
         * @member {number|null|undefined} quantity
         * @memberof proto.InventoryMoveSpec
         * @instance
         */
        InventoryMoveSpec.prototype.quantity = null;

        /**
         * InventoryMoveSpec allowSwapOrMerge.
         * @member {boolean} allowSwapOrMerge
         * @memberof proto.InventoryMoveSpec
         * @instance
         */
        InventoryMoveSpec.prototype.allowSwapOrMerge = false;

        // OneOf field names bound to virtual getters and setters
        let $oneOfFields;

        // Virtual OneOf for proto3 optional field
        Object.defineProperty(InventoryMoveSpec.prototype, "_dstPos", {
            get: $util.oneOfGetter($oneOfFields = ["dstPos"]),
            set: $util.oneOfSetter($oneOfFields)
        });

        // Virtual OneOf for proto3 optional field
        Object.defineProperty(InventoryMoveSpec.prototype, "_dstEquipSlot", {
            get: $util.oneOfGetter($oneOfFields = ["dstEquipSlot"]),
            set: $util.oneOfSetter($oneOfFields)
        });

        // Virtual OneOf for proto3 optional field
        Object.defineProperty(InventoryMoveSpec.prototype, "_quantity", {
            get: $util.oneOfGetter($oneOfFields = ["quantity"]),
            set: $util.oneOfSetter($oneOfFields)
        });

        /**
         * Creates a new InventoryMoveSpec instance using the specified properties.
         * @function create
         * @memberof proto.InventoryMoveSpec
         * @static
         * @param {proto.IInventoryMoveSpec=} [properties] Properties to set
         * @returns {proto.InventoryMoveSpec} InventoryMoveSpec instance
         */
        InventoryMoveSpec.create = function create(properties) {
            return new InventoryMoveSpec(properties);
        };

        /**
         * Encodes the specified InventoryMoveSpec message. Does not implicitly {@link proto.InventoryMoveSpec.verify|verify} messages.
         * @function encode
         * @memberof proto.InventoryMoveSpec
         * @static
         * @param {proto.IInventoryMoveSpec} message InventoryMoveSpec message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        InventoryMoveSpec.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.src != null && Object.hasOwnProperty.call(message, "src"))
                $root.proto.InventoryRef.encode(message.src, writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
            if (message.dst != null && Object.hasOwnProperty.call(message, "dst"))
                $root.proto.InventoryRef.encode(message.dst, writer.uint32(/* id 2, wireType 2 =*/18).fork()).ldelim();
            if (message.itemId != null && Object.hasOwnProperty.call(message, "itemId"))
                writer.uint32(/* id 3, wireType 0 =*/24).uint64(message.itemId);
            if (message.dstPos != null && Object.hasOwnProperty.call(message, "dstPos"))
                $root.proto.GridPos.encode(message.dstPos, writer.uint32(/* id 4, wireType 2 =*/34).fork()).ldelim();
            if (message.dstEquipSlot != null && Object.hasOwnProperty.call(message, "dstEquipSlot"))
                writer.uint32(/* id 5, wireType 0 =*/40).int32(message.dstEquipSlot);
            if (message.quantity != null && Object.hasOwnProperty.call(message, "quantity"))
                writer.uint32(/* id 6, wireType 0 =*/48).uint32(message.quantity);
            if (message.allowSwapOrMerge != null && Object.hasOwnProperty.call(message, "allowSwapOrMerge"))
                writer.uint32(/* id 7, wireType 0 =*/56).bool(message.allowSwapOrMerge);
            return writer;
        };

        /**
         * Encodes the specified InventoryMoveSpec message, length delimited. Does not implicitly {@link proto.InventoryMoveSpec.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.InventoryMoveSpec
         * @static
         * @param {proto.IInventoryMoveSpec} message InventoryMoveSpec message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        InventoryMoveSpec.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes an InventoryMoveSpec message from the specified reader or buffer.
         * @function decode
         * @memberof proto.InventoryMoveSpec
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.InventoryMoveSpec} InventoryMoveSpec
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        InventoryMoveSpec.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.InventoryMoveSpec();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.src = $root.proto.InventoryRef.decode(reader, reader.uint32());
                        break;
                    }
                case 2: {
                        message.dst = $root.proto.InventoryRef.decode(reader, reader.uint32());
                        break;
                    }
                case 3: {
                        message.itemId = reader.uint64();
                        break;
                    }
                case 4: {
                        message.dstPos = $root.proto.GridPos.decode(reader, reader.uint32());
                        break;
                    }
                case 5: {
                        message.dstEquipSlot = reader.int32();
                        break;
                    }
                case 6: {
                        message.quantity = reader.uint32();
                        break;
                    }
                case 7: {
                        message.allowSwapOrMerge = reader.bool();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes an InventoryMoveSpec message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.InventoryMoveSpec
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.InventoryMoveSpec} InventoryMoveSpec
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        InventoryMoveSpec.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies an InventoryMoveSpec message.
         * @function verify
         * @memberof proto.InventoryMoveSpec
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        InventoryMoveSpec.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            let properties = {};
            if (message.src != null && message.hasOwnProperty("src")) {
                let error = $root.proto.InventoryRef.verify(message.src);
                if (error)
                    return "src." + error;
            }
            if (message.dst != null && message.hasOwnProperty("dst")) {
                let error = $root.proto.InventoryRef.verify(message.dst);
                if (error)
                    return "dst." + error;
            }
            if (message.itemId != null && message.hasOwnProperty("itemId"))
                if (!$util.isInteger(message.itemId) && !(message.itemId && $util.isInteger(message.itemId.low) && $util.isInteger(message.itemId.high)))
                    return "itemId: integer|Long expected";
            if (message.dstPos != null && message.hasOwnProperty("dstPos")) {
                properties._dstPos = 1;
                {
                    let error = $root.proto.GridPos.verify(message.dstPos);
                    if (error)
                        return "dstPos." + error;
                }
            }
            if (message.dstEquipSlot != null && message.hasOwnProperty("dstEquipSlot")) {
                properties._dstEquipSlot = 1;
                switch (message.dstEquipSlot) {
                default:
                    return "dstEquipSlot: enum value expected";
                case 0:
                case 1:
                case 2:
                case 3:
                case 4:
                case 5:
                case 6:
                case 7:
                case 8:
                case 9:
                case 10:
                case 11:
                    break;
                }
            }
            if (message.quantity != null && message.hasOwnProperty("quantity")) {
                properties._quantity = 1;
                if (!$util.isInteger(message.quantity))
                    return "quantity: integer expected";
            }
            if (message.allowSwapOrMerge != null && message.hasOwnProperty("allowSwapOrMerge"))
                if (typeof message.allowSwapOrMerge !== "boolean")
                    return "allowSwapOrMerge: boolean expected";
            return null;
        };

        /**
         * Creates an InventoryMoveSpec message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.InventoryMoveSpec
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.InventoryMoveSpec} InventoryMoveSpec
         */
        InventoryMoveSpec.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.InventoryMoveSpec)
                return object;
            let message = new $root.proto.InventoryMoveSpec();
            if (object.src != null) {
                if (typeof object.src !== "object")
                    throw TypeError(".proto.InventoryMoveSpec.src: object expected");
                message.src = $root.proto.InventoryRef.fromObject(object.src);
            }
            if (object.dst != null) {
                if (typeof object.dst !== "object")
                    throw TypeError(".proto.InventoryMoveSpec.dst: object expected");
                message.dst = $root.proto.InventoryRef.fromObject(object.dst);
            }
            if (object.itemId != null)
                if ($util.Long)
                    (message.itemId = $util.Long.fromValue(object.itemId)).unsigned = true;
                else if (typeof object.itemId === "string")
                    message.itemId = parseInt(object.itemId, 10);
                else if (typeof object.itemId === "number")
                    message.itemId = object.itemId;
                else if (typeof object.itemId === "object")
                    message.itemId = new $util.LongBits(object.itemId.low >>> 0, object.itemId.high >>> 0).toNumber(true);
            if (object.dstPos != null) {
                if (typeof object.dstPos !== "object")
                    throw TypeError(".proto.InventoryMoveSpec.dstPos: object expected");
                message.dstPos = $root.proto.GridPos.fromObject(object.dstPos);
            }
            switch (object.dstEquipSlot) {
            default:
                if (typeof object.dstEquipSlot === "number") {
                    message.dstEquipSlot = object.dstEquipSlot;
                    break;
                }
                break;
            case "EQUIP_SLOT_NONE":
            case 0:
                message.dstEquipSlot = 0;
                break;
            case "EQUIP_SLOT_HEAD":
            case 1:
                message.dstEquipSlot = 1;
                break;
            case "EQUIP_SLOT_CHEST":
            case 2:
                message.dstEquipSlot = 2;
                break;
            case "EQUIP_SLOT_LEGS":
            case 3:
                message.dstEquipSlot = 3;
                break;
            case "EQUIP_SLOT_FEET":
            case 4:
                message.dstEquipSlot = 4;
                break;
            case "EQUIP_SLOT_HANDS":
            case 5:
                message.dstEquipSlot = 5;
                break;
            case "EQUIP_SLOT_LEFT_HAND":
            case 6:
                message.dstEquipSlot = 6;
                break;
            case "EQUIP_SLOT_RIGHT_HAND":
            case 7:
                message.dstEquipSlot = 7;
                break;
            case "EQUIP_SLOT_BACK":
            case 8:
                message.dstEquipSlot = 8;
                break;
            case "EQUIP_SLOT_NECK":
            case 9:
                message.dstEquipSlot = 9;
                break;
            case "EQUIP_SLOT_RING_1":
            case 10:
                message.dstEquipSlot = 10;
                break;
            case "EQUIP_SLOT_RING_2":
            case 11:
                message.dstEquipSlot = 11;
                break;
            }
            if (object.quantity != null)
                message.quantity = object.quantity >>> 0;
            if (object.allowSwapOrMerge != null)
                message.allowSwapOrMerge = Boolean(object.allowSwapOrMerge);
            return message;
        };

        /**
         * Creates a plain object from an InventoryMoveSpec message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.InventoryMoveSpec
         * @static
         * @param {proto.InventoryMoveSpec} message InventoryMoveSpec
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        InventoryMoveSpec.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.src = null;
                object.dst = null;
                if ($util.Long) {
                    let long = new $util.Long(0, 0, true);
                    object.itemId = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.itemId = options.longs === String ? "0" : 0;
                object.allowSwapOrMerge = false;
            }
            if (message.src != null && message.hasOwnProperty("src"))
                object.src = $root.proto.InventoryRef.toObject(message.src, options);
            if (message.dst != null && message.hasOwnProperty("dst"))
                object.dst = $root.proto.InventoryRef.toObject(message.dst, options);
            if (message.itemId != null && message.hasOwnProperty("itemId"))
                if (typeof message.itemId === "number")
                    object.itemId = options.longs === String ? String(message.itemId) : message.itemId;
                else
                    object.itemId = options.longs === String ? $util.Long.prototype.toString.call(message.itemId) : options.longs === Number ? new $util.LongBits(message.itemId.low >>> 0, message.itemId.high >>> 0).toNumber(true) : message.itemId;
            if (message.dstPos != null && message.hasOwnProperty("dstPos")) {
                object.dstPos = $root.proto.GridPos.toObject(message.dstPos, options);
                if (options.oneofs)
                    object._dstPos = "dstPos";
            }
            if (message.dstEquipSlot != null && message.hasOwnProperty("dstEquipSlot")) {
                object.dstEquipSlot = options.enums === String ? $root.proto.EquipSlot[message.dstEquipSlot] === undefined ? message.dstEquipSlot : $root.proto.EquipSlot[message.dstEquipSlot] : message.dstEquipSlot;
                if (options.oneofs)
                    object._dstEquipSlot = "dstEquipSlot";
            }
            if (message.quantity != null && message.hasOwnProperty("quantity")) {
                object.quantity = message.quantity;
                if (options.oneofs)
                    object._quantity = "quantity";
            }
            if (message.allowSwapOrMerge != null && message.hasOwnProperty("allowSwapOrMerge"))
                object.allowSwapOrMerge = message.allowSwapOrMerge;
            return object;
        };

        /**
         * Converts this InventoryMoveSpec to JSON.
         * @function toJSON
         * @memberof proto.InventoryMoveSpec
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        InventoryMoveSpec.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for InventoryMoveSpec
         * @function getTypeUrl
         * @memberof proto.InventoryMoveSpec
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        InventoryMoveSpec.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.InventoryMoveSpec";
        };

        return InventoryMoveSpec;
    })();

    proto.InventoryOp = (function() {

        /**
         * Properties of an InventoryOp.
         * @memberof proto
         * @interface IInventoryOp
         * @property {number|Long|null} [opId] InventoryOp opId
         * @property {Array.<proto.IInventoryExpected>|null} [expected] InventoryOp expected
         * @property {proto.IInventoryMoveSpec|null} [move] InventoryOp move
         * @property {proto.IInventoryMoveSpec|null} [dropToWorld] InventoryOp dropToWorld
         * @property {proto.IInventoryMoveSpec|null} [pickupFromWorld] InventoryOp pickupFromWorld
         */

        /**
         * Constructs a new InventoryOp.
         * @memberof proto
         * @classdesc Represents an InventoryOp.
         * @implements IInventoryOp
         * @constructor
         * @param {proto.IInventoryOp=} [properties] Properties to set
         */
        function InventoryOp(properties) {
            this.expected = [];
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * InventoryOp opId.
         * @member {number|Long} opId
         * @memberof proto.InventoryOp
         * @instance
         */
        InventoryOp.prototype.opId = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * InventoryOp expected.
         * @member {Array.<proto.IInventoryExpected>} expected
         * @memberof proto.InventoryOp
         * @instance
         */
        InventoryOp.prototype.expected = $util.emptyArray;

        /**
         * InventoryOp move.
         * @member {proto.IInventoryMoveSpec|null|undefined} move
         * @memberof proto.InventoryOp
         * @instance
         */
        InventoryOp.prototype.move = null;

        /**
         * InventoryOp dropToWorld.
         * @member {proto.IInventoryMoveSpec|null|undefined} dropToWorld
         * @memberof proto.InventoryOp
         * @instance
         */
        InventoryOp.prototype.dropToWorld = null;

        /**
         * InventoryOp pickupFromWorld.
         * @member {proto.IInventoryMoveSpec|null|undefined} pickupFromWorld
         * @memberof proto.InventoryOp
         * @instance
         */
        InventoryOp.prototype.pickupFromWorld = null;

        // OneOf field names bound to virtual getters and setters
        let $oneOfFields;

        /**
         * InventoryOp kind.
         * @member {"move"|"dropToWorld"|"pickupFromWorld"|undefined} kind
         * @memberof proto.InventoryOp
         * @instance
         */
        Object.defineProperty(InventoryOp.prototype, "kind", {
            get: $util.oneOfGetter($oneOfFields = ["move", "dropToWorld", "pickupFromWorld"]),
            set: $util.oneOfSetter($oneOfFields)
        });

        /**
         * Creates a new InventoryOp instance using the specified properties.
         * @function create
         * @memberof proto.InventoryOp
         * @static
         * @param {proto.IInventoryOp=} [properties] Properties to set
         * @returns {proto.InventoryOp} InventoryOp instance
         */
        InventoryOp.create = function create(properties) {
            return new InventoryOp(properties);
        };

        /**
         * Encodes the specified InventoryOp message. Does not implicitly {@link proto.InventoryOp.verify|verify} messages.
         * @function encode
         * @memberof proto.InventoryOp
         * @static
         * @param {proto.IInventoryOp} message InventoryOp message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        InventoryOp.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.opId != null && Object.hasOwnProperty.call(message, "opId"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint64(message.opId);
            if (message.expected != null && message.expected.length)
                for (let i = 0; i < message.expected.length; ++i)
                    $root.proto.InventoryExpected.encode(message.expected[i], writer.uint32(/* id 2, wireType 2 =*/18).fork()).ldelim();
            if (message.move != null && Object.hasOwnProperty.call(message, "move"))
                $root.proto.InventoryMoveSpec.encode(message.move, writer.uint32(/* id 10, wireType 2 =*/82).fork()).ldelim();
            if (message.dropToWorld != null && Object.hasOwnProperty.call(message, "dropToWorld"))
                $root.proto.InventoryMoveSpec.encode(message.dropToWorld, writer.uint32(/* id 12, wireType 2 =*/98).fork()).ldelim();
            if (message.pickupFromWorld != null && Object.hasOwnProperty.call(message, "pickupFromWorld"))
                $root.proto.InventoryMoveSpec.encode(message.pickupFromWorld, writer.uint32(/* id 13, wireType 2 =*/106).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified InventoryOp message, length delimited. Does not implicitly {@link proto.InventoryOp.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.InventoryOp
         * @static
         * @param {proto.IInventoryOp} message InventoryOp message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        InventoryOp.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes an InventoryOp message from the specified reader or buffer.
         * @function decode
         * @memberof proto.InventoryOp
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.InventoryOp} InventoryOp
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        InventoryOp.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.InventoryOp();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.opId = reader.uint64();
                        break;
                    }
                case 2: {
                        if (!(message.expected && message.expected.length))
                            message.expected = [];
                        message.expected.push($root.proto.InventoryExpected.decode(reader, reader.uint32()));
                        break;
                    }
                case 10: {
                        message.move = $root.proto.InventoryMoveSpec.decode(reader, reader.uint32());
                        break;
                    }
                case 12: {
                        message.dropToWorld = $root.proto.InventoryMoveSpec.decode(reader, reader.uint32());
                        break;
                    }
                case 13: {
                        message.pickupFromWorld = $root.proto.InventoryMoveSpec.decode(reader, reader.uint32());
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes an InventoryOp message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.InventoryOp
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.InventoryOp} InventoryOp
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        InventoryOp.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies an InventoryOp message.
         * @function verify
         * @memberof proto.InventoryOp
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        InventoryOp.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            let properties = {};
            if (message.opId != null && message.hasOwnProperty("opId"))
                if (!$util.isInteger(message.opId) && !(message.opId && $util.isInteger(message.opId.low) && $util.isInteger(message.opId.high)))
                    return "opId: integer|Long expected";
            if (message.expected != null && message.hasOwnProperty("expected")) {
                if (!Array.isArray(message.expected))
                    return "expected: array expected";
                for (let i = 0; i < message.expected.length; ++i) {
                    let error = $root.proto.InventoryExpected.verify(message.expected[i]);
                    if (error)
                        return "expected." + error;
                }
            }
            if (message.move != null && message.hasOwnProperty("move")) {
                properties.kind = 1;
                {
                    let error = $root.proto.InventoryMoveSpec.verify(message.move);
                    if (error)
                        return "move." + error;
                }
            }
            if (message.dropToWorld != null && message.hasOwnProperty("dropToWorld")) {
                if (properties.kind === 1)
                    return "kind: multiple values";
                properties.kind = 1;
                {
                    let error = $root.proto.InventoryMoveSpec.verify(message.dropToWorld);
                    if (error)
                        return "dropToWorld." + error;
                }
            }
            if (message.pickupFromWorld != null && message.hasOwnProperty("pickupFromWorld")) {
                if (properties.kind === 1)
                    return "kind: multiple values";
                properties.kind = 1;
                {
                    let error = $root.proto.InventoryMoveSpec.verify(message.pickupFromWorld);
                    if (error)
                        return "pickupFromWorld." + error;
                }
            }
            return null;
        };

        /**
         * Creates an InventoryOp message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.InventoryOp
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.InventoryOp} InventoryOp
         */
        InventoryOp.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.InventoryOp)
                return object;
            let message = new $root.proto.InventoryOp();
            if (object.opId != null)
                if ($util.Long)
                    (message.opId = $util.Long.fromValue(object.opId)).unsigned = true;
                else if (typeof object.opId === "string")
                    message.opId = parseInt(object.opId, 10);
                else if (typeof object.opId === "number")
                    message.opId = object.opId;
                else if (typeof object.opId === "object")
                    message.opId = new $util.LongBits(object.opId.low >>> 0, object.opId.high >>> 0).toNumber(true);
            if (object.expected) {
                if (!Array.isArray(object.expected))
                    throw TypeError(".proto.InventoryOp.expected: array expected");
                message.expected = [];
                for (let i = 0; i < object.expected.length; ++i) {
                    if (typeof object.expected[i] !== "object")
                        throw TypeError(".proto.InventoryOp.expected: object expected");
                    message.expected[i] = $root.proto.InventoryExpected.fromObject(object.expected[i]);
                }
            }
            if (object.move != null) {
                if (typeof object.move !== "object")
                    throw TypeError(".proto.InventoryOp.move: object expected");
                message.move = $root.proto.InventoryMoveSpec.fromObject(object.move);
            }
            if (object.dropToWorld != null) {
                if (typeof object.dropToWorld !== "object")
                    throw TypeError(".proto.InventoryOp.dropToWorld: object expected");
                message.dropToWorld = $root.proto.InventoryMoveSpec.fromObject(object.dropToWorld);
            }
            if (object.pickupFromWorld != null) {
                if (typeof object.pickupFromWorld !== "object")
                    throw TypeError(".proto.InventoryOp.pickupFromWorld: object expected");
                message.pickupFromWorld = $root.proto.InventoryMoveSpec.fromObject(object.pickupFromWorld);
            }
            return message;
        };

        /**
         * Creates a plain object from an InventoryOp message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.InventoryOp
         * @static
         * @param {proto.InventoryOp} message InventoryOp
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        InventoryOp.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.arrays || options.defaults)
                object.expected = [];
            if (options.defaults)
                if ($util.Long) {
                    let long = new $util.Long(0, 0, true);
                    object.opId = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.opId = options.longs === String ? "0" : 0;
            if (message.opId != null && message.hasOwnProperty("opId"))
                if (typeof message.opId === "number")
                    object.opId = options.longs === String ? String(message.opId) : message.opId;
                else
                    object.opId = options.longs === String ? $util.Long.prototype.toString.call(message.opId) : options.longs === Number ? new $util.LongBits(message.opId.low >>> 0, message.opId.high >>> 0).toNumber(true) : message.opId;
            if (message.expected && message.expected.length) {
                object.expected = [];
                for (let j = 0; j < message.expected.length; ++j)
                    object.expected[j] = $root.proto.InventoryExpected.toObject(message.expected[j], options);
            }
            if (message.move != null && message.hasOwnProperty("move")) {
                object.move = $root.proto.InventoryMoveSpec.toObject(message.move, options);
                if (options.oneofs)
                    object.kind = "move";
            }
            if (message.dropToWorld != null && message.hasOwnProperty("dropToWorld")) {
                object.dropToWorld = $root.proto.InventoryMoveSpec.toObject(message.dropToWorld, options);
                if (options.oneofs)
                    object.kind = "dropToWorld";
            }
            if (message.pickupFromWorld != null && message.hasOwnProperty("pickupFromWorld")) {
                object.pickupFromWorld = $root.proto.InventoryMoveSpec.toObject(message.pickupFromWorld, options);
                if (options.oneofs)
                    object.kind = "pickupFromWorld";
            }
            return object;
        };

        /**
         * Converts this InventoryOp to JSON.
         * @function toJSON
         * @memberof proto.InventoryOp
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        InventoryOp.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for InventoryOp
         * @function getTypeUrl
         * @memberof proto.InventoryOp
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        InventoryOp.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.InventoryOp";
        };

        return InventoryOp;
    })();

    proto.C2S_InventoryOp = (function() {

        /**
         * Properties of a C2S_InventoryOp.
         * @memberof proto
         * @interface IC2S_InventoryOp
         * @property {proto.IInventoryOp|null} [op] C2S_InventoryOp op
         */

        /**
         * Constructs a new C2S_InventoryOp.
         * @memberof proto
         * @classdesc Represents a C2S_InventoryOp.
         * @implements IC2S_InventoryOp
         * @constructor
         * @param {proto.IC2S_InventoryOp=} [properties] Properties to set
         */
        function C2S_InventoryOp(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * C2S_InventoryOp op.
         * @member {proto.IInventoryOp|null|undefined} op
         * @memberof proto.C2S_InventoryOp
         * @instance
         */
        C2S_InventoryOp.prototype.op = null;

        /**
         * Creates a new C2S_InventoryOp instance using the specified properties.
         * @function create
         * @memberof proto.C2S_InventoryOp
         * @static
         * @param {proto.IC2S_InventoryOp=} [properties] Properties to set
         * @returns {proto.C2S_InventoryOp} C2S_InventoryOp instance
         */
        C2S_InventoryOp.create = function create(properties) {
            return new C2S_InventoryOp(properties);
        };

        /**
         * Encodes the specified C2S_InventoryOp message. Does not implicitly {@link proto.C2S_InventoryOp.verify|verify} messages.
         * @function encode
         * @memberof proto.C2S_InventoryOp
         * @static
         * @param {proto.IC2S_InventoryOp} message C2S_InventoryOp message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        C2S_InventoryOp.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.op != null && Object.hasOwnProperty.call(message, "op"))
                $root.proto.InventoryOp.encode(message.op, writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified C2S_InventoryOp message, length delimited. Does not implicitly {@link proto.C2S_InventoryOp.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.C2S_InventoryOp
         * @static
         * @param {proto.IC2S_InventoryOp} message C2S_InventoryOp message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        C2S_InventoryOp.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a C2S_InventoryOp message from the specified reader or buffer.
         * @function decode
         * @memberof proto.C2S_InventoryOp
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.C2S_InventoryOp} C2S_InventoryOp
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        C2S_InventoryOp.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.C2S_InventoryOp();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.op = $root.proto.InventoryOp.decode(reader, reader.uint32());
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a C2S_InventoryOp message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.C2S_InventoryOp
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.C2S_InventoryOp} C2S_InventoryOp
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        C2S_InventoryOp.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a C2S_InventoryOp message.
         * @function verify
         * @memberof proto.C2S_InventoryOp
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        C2S_InventoryOp.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.op != null && message.hasOwnProperty("op")) {
                let error = $root.proto.InventoryOp.verify(message.op);
                if (error)
                    return "op." + error;
            }
            return null;
        };

        /**
         * Creates a C2S_InventoryOp message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.C2S_InventoryOp
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.C2S_InventoryOp} C2S_InventoryOp
         */
        C2S_InventoryOp.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.C2S_InventoryOp)
                return object;
            let message = new $root.proto.C2S_InventoryOp();
            if (object.op != null) {
                if (typeof object.op !== "object")
                    throw TypeError(".proto.C2S_InventoryOp.op: object expected");
                message.op = $root.proto.InventoryOp.fromObject(object.op);
            }
            return message;
        };

        /**
         * Creates a plain object from a C2S_InventoryOp message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.C2S_InventoryOp
         * @static
         * @param {proto.C2S_InventoryOp} message C2S_InventoryOp
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        C2S_InventoryOp.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults)
                object.op = null;
            if (message.op != null && message.hasOwnProperty("op"))
                object.op = $root.proto.InventoryOp.toObject(message.op, options);
            return object;
        };

        /**
         * Converts this C2S_InventoryOp to JSON.
         * @function toJSON
         * @memberof proto.C2S_InventoryOp
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        C2S_InventoryOp.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for C2S_InventoryOp
         * @function getTypeUrl
         * @memberof proto.C2S_InventoryOp
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        C2S_InventoryOp.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.C2S_InventoryOp";
        };

        return C2S_InventoryOp;
    })();

    proto.EntityMovement = (function() {

        /**
         * Properties of an EntityMovement.
         * @memberof proto
         * @interface IEntityMovement
         * @property {proto.IPosition|null} [position] EntityMovement position
         * @property {proto.IVector2|null} [velocity] EntityMovement velocity
         * @property {proto.MovementMode|null} [moveMode] EntityMovement moveMode
         * @property {proto.IVector2|null} [targetPosition] EntityMovement targetPosition
         * @property {boolean|null} [isMoving] EntityMovement isMoving
         */

        /**
         * Constructs a new EntityMovement.
         * @memberof proto
         * @classdesc Represents an EntityMovement.
         * @implements IEntityMovement
         * @constructor
         * @param {proto.IEntityMovement=} [properties] Properties to set
         */
        function EntityMovement(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * EntityMovement position.
         * @member {proto.IPosition|null|undefined} position
         * @memberof proto.EntityMovement
         * @instance
         */
        EntityMovement.prototype.position = null;

        /**
         * EntityMovement velocity.
         * @member {proto.IVector2|null|undefined} velocity
         * @memberof proto.EntityMovement
         * @instance
         */
        EntityMovement.prototype.velocity = null;

        /**
         * EntityMovement moveMode.
         * @member {proto.MovementMode} moveMode
         * @memberof proto.EntityMovement
         * @instance
         */
        EntityMovement.prototype.moveMode = 0;

        /**
         * EntityMovement targetPosition.
         * @member {proto.IVector2|null|undefined} targetPosition
         * @memberof proto.EntityMovement
         * @instance
         */
        EntityMovement.prototype.targetPosition = null;

        /**
         * EntityMovement isMoving.
         * @member {boolean} isMoving
         * @memberof proto.EntityMovement
         * @instance
         */
        EntityMovement.prototype.isMoving = false;

        // OneOf field names bound to virtual getters and setters
        let $oneOfFields;

        // Virtual OneOf for proto3 optional field
        Object.defineProperty(EntityMovement.prototype, "_targetPosition", {
            get: $util.oneOfGetter($oneOfFields = ["targetPosition"]),
            set: $util.oneOfSetter($oneOfFields)
        });

        /**
         * Creates a new EntityMovement instance using the specified properties.
         * @function create
         * @memberof proto.EntityMovement
         * @static
         * @param {proto.IEntityMovement=} [properties] Properties to set
         * @returns {proto.EntityMovement} EntityMovement instance
         */
        EntityMovement.create = function create(properties) {
            return new EntityMovement(properties);
        };

        /**
         * Encodes the specified EntityMovement message. Does not implicitly {@link proto.EntityMovement.verify|verify} messages.
         * @function encode
         * @memberof proto.EntityMovement
         * @static
         * @param {proto.IEntityMovement} message EntityMovement message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        EntityMovement.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.position != null && Object.hasOwnProperty.call(message, "position"))
                $root.proto.Position.encode(message.position, writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
            if (message.velocity != null && Object.hasOwnProperty.call(message, "velocity"))
                $root.proto.Vector2.encode(message.velocity, writer.uint32(/* id 2, wireType 2 =*/18).fork()).ldelim();
            if (message.moveMode != null && Object.hasOwnProperty.call(message, "moveMode"))
                writer.uint32(/* id 4, wireType 0 =*/32).int32(message.moveMode);
            if (message.targetPosition != null && Object.hasOwnProperty.call(message, "targetPosition"))
                $root.proto.Vector2.encode(message.targetPosition, writer.uint32(/* id 5, wireType 2 =*/42).fork()).ldelim();
            if (message.isMoving != null && Object.hasOwnProperty.call(message, "isMoving"))
                writer.uint32(/* id 6, wireType 0 =*/48).bool(message.isMoving);
            return writer;
        };

        /**
         * Encodes the specified EntityMovement message, length delimited. Does not implicitly {@link proto.EntityMovement.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.EntityMovement
         * @static
         * @param {proto.IEntityMovement} message EntityMovement message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        EntityMovement.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes an EntityMovement message from the specified reader or buffer.
         * @function decode
         * @memberof proto.EntityMovement
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.EntityMovement} EntityMovement
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        EntityMovement.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.EntityMovement();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.position = $root.proto.Position.decode(reader, reader.uint32());
                        break;
                    }
                case 2: {
                        message.velocity = $root.proto.Vector2.decode(reader, reader.uint32());
                        break;
                    }
                case 4: {
                        message.moveMode = reader.int32();
                        break;
                    }
                case 5: {
                        message.targetPosition = $root.proto.Vector2.decode(reader, reader.uint32());
                        break;
                    }
                case 6: {
                        message.isMoving = reader.bool();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes an EntityMovement message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.EntityMovement
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.EntityMovement} EntityMovement
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        EntityMovement.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies an EntityMovement message.
         * @function verify
         * @memberof proto.EntityMovement
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        EntityMovement.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            let properties = {};
            if (message.position != null && message.hasOwnProperty("position")) {
                let error = $root.proto.Position.verify(message.position);
                if (error)
                    return "position." + error;
            }
            if (message.velocity != null && message.hasOwnProperty("velocity")) {
                let error = $root.proto.Vector2.verify(message.velocity);
                if (error)
                    return "velocity." + error;
            }
            if (message.moveMode != null && message.hasOwnProperty("moveMode"))
                switch (message.moveMode) {
                default:
                    return "moveMode: enum value expected";
                case 0:
                case 1:
                case 2:
                case 3:
                    break;
                }
            if (message.targetPosition != null && message.hasOwnProperty("targetPosition")) {
                properties._targetPosition = 1;
                {
                    let error = $root.proto.Vector2.verify(message.targetPosition);
                    if (error)
                        return "targetPosition." + error;
                }
            }
            if (message.isMoving != null && message.hasOwnProperty("isMoving"))
                if (typeof message.isMoving !== "boolean")
                    return "isMoving: boolean expected";
            return null;
        };

        /**
         * Creates an EntityMovement message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.EntityMovement
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.EntityMovement} EntityMovement
         */
        EntityMovement.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.EntityMovement)
                return object;
            let message = new $root.proto.EntityMovement();
            if (object.position != null) {
                if (typeof object.position !== "object")
                    throw TypeError(".proto.EntityMovement.position: object expected");
                message.position = $root.proto.Position.fromObject(object.position);
            }
            if (object.velocity != null) {
                if (typeof object.velocity !== "object")
                    throw TypeError(".proto.EntityMovement.velocity: object expected");
                message.velocity = $root.proto.Vector2.fromObject(object.velocity);
            }
            switch (object.moveMode) {
            default:
                if (typeof object.moveMode === "number") {
                    message.moveMode = object.moveMode;
                    break;
                }
                break;
            case "MOVE_MODE_WALK":
            case 0:
                message.moveMode = 0;
                break;
            case "MOVE_MODE_RUN":
            case 1:
                message.moveMode = 1;
                break;
            case "MOVE_MODE_FAST_RUN":
            case 2:
                message.moveMode = 2;
                break;
            case "MOVE_MODE_SWIM":
            case 3:
                message.moveMode = 3;
                break;
            }
            if (object.targetPosition != null) {
                if (typeof object.targetPosition !== "object")
                    throw TypeError(".proto.EntityMovement.targetPosition: object expected");
                message.targetPosition = $root.proto.Vector2.fromObject(object.targetPosition);
            }
            if (object.isMoving != null)
                message.isMoving = Boolean(object.isMoving);
            return message;
        };

        /**
         * Creates a plain object from an EntityMovement message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.EntityMovement
         * @static
         * @param {proto.EntityMovement} message EntityMovement
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        EntityMovement.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.position = null;
                object.velocity = null;
                object.moveMode = options.enums === String ? "MOVE_MODE_WALK" : 0;
                object.isMoving = false;
            }
            if (message.position != null && message.hasOwnProperty("position"))
                object.position = $root.proto.Position.toObject(message.position, options);
            if (message.velocity != null && message.hasOwnProperty("velocity"))
                object.velocity = $root.proto.Vector2.toObject(message.velocity, options);
            if (message.moveMode != null && message.hasOwnProperty("moveMode"))
                object.moveMode = options.enums === String ? $root.proto.MovementMode[message.moveMode] === undefined ? message.moveMode : $root.proto.MovementMode[message.moveMode] : message.moveMode;
            if (message.targetPosition != null && message.hasOwnProperty("targetPosition")) {
                object.targetPosition = $root.proto.Vector2.toObject(message.targetPosition, options);
                if (options.oneofs)
                    object._targetPosition = "targetPosition";
            }
            if (message.isMoving != null && message.hasOwnProperty("isMoving"))
                object.isMoving = message.isMoving;
            return object;
        };

        /**
         * Converts this EntityMovement to JSON.
         * @function toJSON
         * @memberof proto.EntityMovement
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        EntityMovement.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for EntityMovement
         * @function getTypeUrl
         * @memberof proto.EntityMovement
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        EntityMovement.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.EntityMovement";
        };

        return EntityMovement;
    })();

    proto.EntityPosition = (function() {

        /**
         * Properties of an EntityPosition.
         * @memberof proto
         * @interface IEntityPosition
         * @property {proto.IPosition|null} [position] EntityPosition position
         * @property {proto.IVector2|null} [size] EntityPosition size
         */

        /**
         * Constructs a new EntityPosition.
         * @memberof proto
         * @classdesc Represents an EntityPosition.
         * @implements IEntityPosition
         * @constructor
         * @param {proto.IEntityPosition=} [properties] Properties to set
         */
        function EntityPosition(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * EntityPosition position.
         * @member {proto.IPosition|null|undefined} position
         * @memberof proto.EntityPosition
         * @instance
         */
        EntityPosition.prototype.position = null;

        /**
         * EntityPosition size.
         * @member {proto.IVector2|null|undefined} size
         * @memberof proto.EntityPosition
         * @instance
         */
        EntityPosition.prototype.size = null;

        /**
         * Creates a new EntityPosition instance using the specified properties.
         * @function create
         * @memberof proto.EntityPosition
         * @static
         * @param {proto.IEntityPosition=} [properties] Properties to set
         * @returns {proto.EntityPosition} EntityPosition instance
         */
        EntityPosition.create = function create(properties) {
            return new EntityPosition(properties);
        };

        /**
         * Encodes the specified EntityPosition message. Does not implicitly {@link proto.EntityPosition.verify|verify} messages.
         * @function encode
         * @memberof proto.EntityPosition
         * @static
         * @param {proto.IEntityPosition} message EntityPosition message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        EntityPosition.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.position != null && Object.hasOwnProperty.call(message, "position"))
                $root.proto.Position.encode(message.position, writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
            if (message.size != null && Object.hasOwnProperty.call(message, "size"))
                $root.proto.Vector2.encode(message.size, writer.uint32(/* id 2, wireType 2 =*/18).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified EntityPosition message, length delimited. Does not implicitly {@link proto.EntityPosition.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.EntityPosition
         * @static
         * @param {proto.IEntityPosition} message EntityPosition message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        EntityPosition.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes an EntityPosition message from the specified reader or buffer.
         * @function decode
         * @memberof proto.EntityPosition
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.EntityPosition} EntityPosition
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        EntityPosition.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.EntityPosition();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.position = $root.proto.Position.decode(reader, reader.uint32());
                        break;
                    }
                case 2: {
                        message.size = $root.proto.Vector2.decode(reader, reader.uint32());
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes an EntityPosition message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.EntityPosition
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.EntityPosition} EntityPosition
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        EntityPosition.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies an EntityPosition message.
         * @function verify
         * @memberof proto.EntityPosition
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        EntityPosition.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.position != null && message.hasOwnProperty("position")) {
                let error = $root.proto.Position.verify(message.position);
                if (error)
                    return "position." + error;
            }
            if (message.size != null && message.hasOwnProperty("size")) {
                let error = $root.proto.Vector2.verify(message.size);
                if (error)
                    return "size." + error;
            }
            return null;
        };

        /**
         * Creates an EntityPosition message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.EntityPosition
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.EntityPosition} EntityPosition
         */
        EntityPosition.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.EntityPosition)
                return object;
            let message = new $root.proto.EntityPosition();
            if (object.position != null) {
                if (typeof object.position !== "object")
                    throw TypeError(".proto.EntityPosition.position: object expected");
                message.position = $root.proto.Position.fromObject(object.position);
            }
            if (object.size != null) {
                if (typeof object.size !== "object")
                    throw TypeError(".proto.EntityPosition.size: object expected");
                message.size = $root.proto.Vector2.fromObject(object.size);
            }
            return message;
        };

        /**
         * Creates a plain object from an EntityPosition message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.EntityPosition
         * @static
         * @param {proto.EntityPosition} message EntityPosition
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        EntityPosition.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.position = null;
                object.size = null;
            }
            if (message.position != null && message.hasOwnProperty("position"))
                object.position = $root.proto.Position.toObject(message.position, options);
            if (message.size != null && message.hasOwnProperty("size"))
                object.size = $root.proto.Vector2.toObject(message.size, options);
            return object;
        };

        /**
         * Converts this EntityPosition to JSON.
         * @function toJSON
         * @memberof proto.EntityPosition
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        EntityPosition.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for EntityPosition
         * @function getTypeUrl
         * @memberof proto.EntityPosition
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        EntityPosition.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.EntityPosition";
        };

        return EntityPosition;
    })();

    proto.EntityAppearance = (function() {

        /**
         * Properties of an EntityAppearance.
         * @memberof proto
         * @interface IEntityAppearance
         * @property {string|null} [resource] EntityAppearance resource
         * @property {string|null} [name] EntityAppearance name
         */

        /**
         * Constructs a new EntityAppearance.
         * @memberof proto
         * @classdesc Represents an EntityAppearance.
         * @implements IEntityAppearance
         * @constructor
         * @param {proto.IEntityAppearance=} [properties] Properties to set
         */
        function EntityAppearance(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * EntityAppearance resource.
         * @member {string} resource
         * @memberof proto.EntityAppearance
         * @instance
         */
        EntityAppearance.prototype.resource = "";

        /**
         * EntityAppearance name.
         * @member {string} name
         * @memberof proto.EntityAppearance
         * @instance
         */
        EntityAppearance.prototype.name = "";

        /**
         * Creates a new EntityAppearance instance using the specified properties.
         * @function create
         * @memberof proto.EntityAppearance
         * @static
         * @param {proto.IEntityAppearance=} [properties] Properties to set
         * @returns {proto.EntityAppearance} EntityAppearance instance
         */
        EntityAppearance.create = function create(properties) {
            return new EntityAppearance(properties);
        };

        /**
         * Encodes the specified EntityAppearance message. Does not implicitly {@link proto.EntityAppearance.verify|verify} messages.
         * @function encode
         * @memberof proto.EntityAppearance
         * @static
         * @param {proto.IEntityAppearance} message EntityAppearance message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        EntityAppearance.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.resource != null && Object.hasOwnProperty.call(message, "resource"))
                writer.uint32(/* id 1, wireType 2 =*/10).string(message.resource);
            if (message.name != null && Object.hasOwnProperty.call(message, "name"))
                writer.uint32(/* id 2, wireType 2 =*/18).string(message.name);
            return writer;
        };

        /**
         * Encodes the specified EntityAppearance message, length delimited. Does not implicitly {@link proto.EntityAppearance.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.EntityAppearance
         * @static
         * @param {proto.IEntityAppearance} message EntityAppearance message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        EntityAppearance.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes an EntityAppearance message from the specified reader or buffer.
         * @function decode
         * @memberof proto.EntityAppearance
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.EntityAppearance} EntityAppearance
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        EntityAppearance.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.EntityAppearance();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.resource = reader.string();
                        break;
                    }
                case 2: {
                        message.name = reader.string();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes an EntityAppearance message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.EntityAppearance
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.EntityAppearance} EntityAppearance
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        EntityAppearance.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies an EntityAppearance message.
         * @function verify
         * @memberof proto.EntityAppearance
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        EntityAppearance.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.resource != null && message.hasOwnProperty("resource"))
                if (!$util.isString(message.resource))
                    return "resource: string expected";
            if (message.name != null && message.hasOwnProperty("name"))
                if (!$util.isString(message.name))
                    return "name: string expected";
            return null;
        };

        /**
         * Creates an EntityAppearance message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.EntityAppearance
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.EntityAppearance} EntityAppearance
         */
        EntityAppearance.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.EntityAppearance)
                return object;
            let message = new $root.proto.EntityAppearance();
            if (object.resource != null)
                message.resource = String(object.resource);
            if (object.name != null)
                message.name = String(object.name);
            return message;
        };

        /**
         * Creates a plain object from an EntityAppearance message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.EntityAppearance
         * @static
         * @param {proto.EntityAppearance} message EntityAppearance
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        EntityAppearance.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.resource = "";
                object.name = "";
            }
            if (message.resource != null && message.hasOwnProperty("resource"))
                object.resource = message.resource;
            if (message.name != null && message.hasOwnProperty("name"))
                object.name = message.name;
            return object;
        };

        /**
         * Converts this EntityAppearance to JSON.
         * @function toJSON
         * @memberof proto.EntityAppearance
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        EntityAppearance.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for EntityAppearance
         * @function getTypeUrl
         * @memberof proto.EntityAppearance
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        EntityAppearance.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.EntityAppearance";
        };

        return EntityAppearance;
    })();

    proto.ChunkCoord = (function() {

        /**
         * Properties of a ChunkCoord.
         * @memberof proto
         * @interface IChunkCoord
         * @property {number|null} [x] ChunkCoord x
         * @property {number|null} [y] ChunkCoord y
         */

        /**
         * Constructs a new ChunkCoord.
         * @memberof proto
         * @classdesc Represents a ChunkCoord.
         * @implements IChunkCoord
         * @constructor
         * @param {proto.IChunkCoord=} [properties] Properties to set
         */
        function ChunkCoord(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * ChunkCoord x.
         * @member {number} x
         * @memberof proto.ChunkCoord
         * @instance
         */
        ChunkCoord.prototype.x = 0;

        /**
         * ChunkCoord y.
         * @member {number} y
         * @memberof proto.ChunkCoord
         * @instance
         */
        ChunkCoord.prototype.y = 0;

        /**
         * Creates a new ChunkCoord instance using the specified properties.
         * @function create
         * @memberof proto.ChunkCoord
         * @static
         * @param {proto.IChunkCoord=} [properties] Properties to set
         * @returns {proto.ChunkCoord} ChunkCoord instance
         */
        ChunkCoord.create = function create(properties) {
            return new ChunkCoord(properties);
        };

        /**
         * Encodes the specified ChunkCoord message. Does not implicitly {@link proto.ChunkCoord.verify|verify} messages.
         * @function encode
         * @memberof proto.ChunkCoord
         * @static
         * @param {proto.IChunkCoord} message ChunkCoord message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        ChunkCoord.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.x != null && Object.hasOwnProperty.call(message, "x"))
                writer.uint32(/* id 1, wireType 0 =*/8).int32(message.x);
            if (message.y != null && Object.hasOwnProperty.call(message, "y"))
                writer.uint32(/* id 2, wireType 0 =*/16).int32(message.y);
            return writer;
        };

        /**
         * Encodes the specified ChunkCoord message, length delimited. Does not implicitly {@link proto.ChunkCoord.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.ChunkCoord
         * @static
         * @param {proto.IChunkCoord} message ChunkCoord message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        ChunkCoord.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a ChunkCoord message from the specified reader or buffer.
         * @function decode
         * @memberof proto.ChunkCoord
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.ChunkCoord} ChunkCoord
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        ChunkCoord.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.ChunkCoord();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.x = reader.int32();
                        break;
                    }
                case 2: {
                        message.y = reader.int32();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a ChunkCoord message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.ChunkCoord
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.ChunkCoord} ChunkCoord
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        ChunkCoord.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a ChunkCoord message.
         * @function verify
         * @memberof proto.ChunkCoord
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        ChunkCoord.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.x != null && message.hasOwnProperty("x"))
                if (!$util.isInteger(message.x))
                    return "x: integer expected";
            if (message.y != null && message.hasOwnProperty("y"))
                if (!$util.isInteger(message.y))
                    return "y: integer expected";
            return null;
        };

        /**
         * Creates a ChunkCoord message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.ChunkCoord
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.ChunkCoord} ChunkCoord
         */
        ChunkCoord.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.ChunkCoord)
                return object;
            let message = new $root.proto.ChunkCoord();
            if (object.x != null)
                message.x = object.x | 0;
            if (object.y != null)
                message.y = object.y | 0;
            return message;
        };

        /**
         * Creates a plain object from a ChunkCoord message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.ChunkCoord
         * @static
         * @param {proto.ChunkCoord} message ChunkCoord
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        ChunkCoord.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.x = 0;
                object.y = 0;
            }
            if (message.x != null && message.hasOwnProperty("x"))
                object.x = message.x;
            if (message.y != null && message.hasOwnProperty("y"))
                object.y = message.y;
            return object;
        };

        /**
         * Converts this ChunkCoord to JSON.
         * @function toJSON
         * @memberof proto.ChunkCoord
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        ChunkCoord.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for ChunkCoord
         * @function getTypeUrl
         * @memberof proto.ChunkCoord
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        ChunkCoord.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.ChunkCoord";
        };

        return ChunkCoord;
    })();

    proto.ChunkData = (function() {

        /**
         * Properties of a ChunkData.
         * @memberof proto
         * @interface IChunkData
         * @property {proto.IChunkCoord|null} [coord] ChunkData coord
         * @property {Uint8Array|null} [tiles] ChunkData tiles
         * @property {number|null} [version] ChunkData version
         */

        /**
         * Constructs a new ChunkData.
         * @memberof proto
         * @classdesc Represents a ChunkData.
         * @implements IChunkData
         * @constructor
         * @param {proto.IChunkData=} [properties] Properties to set
         */
        function ChunkData(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * ChunkData coord.
         * @member {proto.IChunkCoord|null|undefined} coord
         * @memberof proto.ChunkData
         * @instance
         */
        ChunkData.prototype.coord = null;

        /**
         * ChunkData tiles.
         * @member {Uint8Array} tiles
         * @memberof proto.ChunkData
         * @instance
         */
        ChunkData.prototype.tiles = $util.newBuffer([]);

        /**
         * ChunkData version.
         * @member {number} version
         * @memberof proto.ChunkData
         * @instance
         */
        ChunkData.prototype.version = 0;

        /**
         * Creates a new ChunkData instance using the specified properties.
         * @function create
         * @memberof proto.ChunkData
         * @static
         * @param {proto.IChunkData=} [properties] Properties to set
         * @returns {proto.ChunkData} ChunkData instance
         */
        ChunkData.create = function create(properties) {
            return new ChunkData(properties);
        };

        /**
         * Encodes the specified ChunkData message. Does not implicitly {@link proto.ChunkData.verify|verify} messages.
         * @function encode
         * @memberof proto.ChunkData
         * @static
         * @param {proto.IChunkData} message ChunkData message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        ChunkData.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.coord != null && Object.hasOwnProperty.call(message, "coord"))
                $root.proto.ChunkCoord.encode(message.coord, writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
            if (message.tiles != null && Object.hasOwnProperty.call(message, "tiles"))
                writer.uint32(/* id 2, wireType 2 =*/18).bytes(message.tiles);
            if (message.version != null && Object.hasOwnProperty.call(message, "version"))
                writer.uint32(/* id 3, wireType 0 =*/24).uint32(message.version);
            return writer;
        };

        /**
         * Encodes the specified ChunkData message, length delimited. Does not implicitly {@link proto.ChunkData.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.ChunkData
         * @static
         * @param {proto.IChunkData} message ChunkData message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        ChunkData.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a ChunkData message from the specified reader or buffer.
         * @function decode
         * @memberof proto.ChunkData
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.ChunkData} ChunkData
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        ChunkData.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.ChunkData();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.coord = $root.proto.ChunkCoord.decode(reader, reader.uint32());
                        break;
                    }
                case 2: {
                        message.tiles = reader.bytes();
                        break;
                    }
                case 3: {
                        message.version = reader.uint32();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a ChunkData message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.ChunkData
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.ChunkData} ChunkData
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        ChunkData.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a ChunkData message.
         * @function verify
         * @memberof proto.ChunkData
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        ChunkData.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.coord != null && message.hasOwnProperty("coord")) {
                let error = $root.proto.ChunkCoord.verify(message.coord);
                if (error)
                    return "coord." + error;
            }
            if (message.tiles != null && message.hasOwnProperty("tiles"))
                if (!(message.tiles && typeof message.tiles.length === "number" || $util.isString(message.tiles)))
                    return "tiles: buffer expected";
            if (message.version != null && message.hasOwnProperty("version"))
                if (!$util.isInteger(message.version))
                    return "version: integer expected";
            return null;
        };

        /**
         * Creates a ChunkData message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.ChunkData
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.ChunkData} ChunkData
         */
        ChunkData.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.ChunkData)
                return object;
            let message = new $root.proto.ChunkData();
            if (object.coord != null) {
                if (typeof object.coord !== "object")
                    throw TypeError(".proto.ChunkData.coord: object expected");
                message.coord = $root.proto.ChunkCoord.fromObject(object.coord);
            }
            if (object.tiles != null)
                if (typeof object.tiles === "string")
                    $util.base64.decode(object.tiles, message.tiles = $util.newBuffer($util.base64.length(object.tiles)), 0);
                else if (object.tiles.length >= 0)
                    message.tiles = object.tiles;
            if (object.version != null)
                message.version = object.version >>> 0;
            return message;
        };

        /**
         * Creates a plain object from a ChunkData message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.ChunkData
         * @static
         * @param {proto.ChunkData} message ChunkData
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        ChunkData.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.coord = null;
                if (options.bytes === String)
                    object.tiles = "";
                else {
                    object.tiles = [];
                    if (options.bytes !== Array)
                        object.tiles = $util.newBuffer(object.tiles);
                }
                object.version = 0;
            }
            if (message.coord != null && message.hasOwnProperty("coord"))
                object.coord = $root.proto.ChunkCoord.toObject(message.coord, options);
            if (message.tiles != null && message.hasOwnProperty("tiles"))
                object.tiles = options.bytes === String ? $util.base64.encode(message.tiles, 0, message.tiles.length) : options.bytes === Array ? Array.prototype.slice.call(message.tiles) : message.tiles;
            if (message.version != null && message.hasOwnProperty("version"))
                object.version = message.version;
            return object;
        };

        /**
         * Converts this ChunkData to JSON.
         * @function toJSON
         * @memberof proto.ChunkData
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        ChunkData.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for ChunkData
         * @function getTypeUrl
         * @memberof proto.ChunkData
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        ChunkData.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.ChunkData";
        };

        return ChunkData;
    })();

    proto.MoveTo = (function() {

        /**
         * Properties of a MoveTo.
         * @memberof proto
         * @interface IMoveTo
         * @property {number|null} [x] MoveTo x
         * @property {number|null} [y] MoveTo y
         */

        /**
         * Constructs a new MoveTo.
         * @memberof proto
         * @classdesc Represents a MoveTo.
         * @implements IMoveTo
         * @constructor
         * @param {proto.IMoveTo=} [properties] Properties to set
         */
        function MoveTo(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * MoveTo x.
         * @member {number} x
         * @memberof proto.MoveTo
         * @instance
         */
        MoveTo.prototype.x = 0;

        /**
         * MoveTo y.
         * @member {number} y
         * @memberof proto.MoveTo
         * @instance
         */
        MoveTo.prototype.y = 0;

        /**
         * Creates a new MoveTo instance using the specified properties.
         * @function create
         * @memberof proto.MoveTo
         * @static
         * @param {proto.IMoveTo=} [properties] Properties to set
         * @returns {proto.MoveTo} MoveTo instance
         */
        MoveTo.create = function create(properties) {
            return new MoveTo(properties);
        };

        /**
         * Encodes the specified MoveTo message. Does not implicitly {@link proto.MoveTo.verify|verify} messages.
         * @function encode
         * @memberof proto.MoveTo
         * @static
         * @param {proto.IMoveTo} message MoveTo message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        MoveTo.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.x != null && Object.hasOwnProperty.call(message, "x"))
                writer.uint32(/* id 1, wireType 0 =*/8).int32(message.x);
            if (message.y != null && Object.hasOwnProperty.call(message, "y"))
                writer.uint32(/* id 2, wireType 0 =*/16).int32(message.y);
            return writer;
        };

        /**
         * Encodes the specified MoveTo message, length delimited. Does not implicitly {@link proto.MoveTo.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.MoveTo
         * @static
         * @param {proto.IMoveTo} message MoveTo message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        MoveTo.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a MoveTo message from the specified reader or buffer.
         * @function decode
         * @memberof proto.MoveTo
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.MoveTo} MoveTo
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        MoveTo.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.MoveTo();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.x = reader.int32();
                        break;
                    }
                case 2: {
                        message.y = reader.int32();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a MoveTo message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.MoveTo
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.MoveTo} MoveTo
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        MoveTo.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a MoveTo message.
         * @function verify
         * @memberof proto.MoveTo
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        MoveTo.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.x != null && message.hasOwnProperty("x"))
                if (!$util.isInteger(message.x))
                    return "x: integer expected";
            if (message.y != null && message.hasOwnProperty("y"))
                if (!$util.isInteger(message.y))
                    return "y: integer expected";
            return null;
        };

        /**
         * Creates a MoveTo message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.MoveTo
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.MoveTo} MoveTo
         */
        MoveTo.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.MoveTo)
                return object;
            let message = new $root.proto.MoveTo();
            if (object.x != null)
                message.x = object.x | 0;
            if (object.y != null)
                message.y = object.y | 0;
            return message;
        };

        /**
         * Creates a plain object from a MoveTo message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.MoveTo
         * @static
         * @param {proto.MoveTo} message MoveTo
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        MoveTo.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.x = 0;
                object.y = 0;
            }
            if (message.x != null && message.hasOwnProperty("x"))
                object.x = message.x;
            if (message.y != null && message.hasOwnProperty("y"))
                object.y = message.y;
            return object;
        };

        /**
         * Converts this MoveTo to JSON.
         * @function toJSON
         * @memberof proto.MoveTo
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        MoveTo.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for MoveTo
         * @function getTypeUrl
         * @memberof proto.MoveTo
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        MoveTo.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.MoveTo";
        };

        return MoveTo;
    })();

    proto.MoveToEntity = (function() {

        /**
         * Properties of a MoveToEntity.
         * @memberof proto
         * @interface IMoveToEntity
         * @property {number|Long|null} [entityId] MoveToEntity entityId
         * @property {boolean|null} [autoInteract] MoveToEntity autoInteract
         */

        /**
         * Constructs a new MoveToEntity.
         * @memberof proto
         * @classdesc Represents a MoveToEntity.
         * @implements IMoveToEntity
         * @constructor
         * @param {proto.IMoveToEntity=} [properties] Properties to set
         */
        function MoveToEntity(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * MoveToEntity entityId.
         * @member {number|Long} entityId
         * @memberof proto.MoveToEntity
         * @instance
         */
        MoveToEntity.prototype.entityId = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * MoveToEntity autoInteract.
         * @member {boolean} autoInteract
         * @memberof proto.MoveToEntity
         * @instance
         */
        MoveToEntity.prototype.autoInteract = false;

        /**
         * Creates a new MoveToEntity instance using the specified properties.
         * @function create
         * @memberof proto.MoveToEntity
         * @static
         * @param {proto.IMoveToEntity=} [properties] Properties to set
         * @returns {proto.MoveToEntity} MoveToEntity instance
         */
        MoveToEntity.create = function create(properties) {
            return new MoveToEntity(properties);
        };

        /**
         * Encodes the specified MoveToEntity message. Does not implicitly {@link proto.MoveToEntity.verify|verify} messages.
         * @function encode
         * @memberof proto.MoveToEntity
         * @static
         * @param {proto.IMoveToEntity} message MoveToEntity message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        MoveToEntity.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.entityId != null && Object.hasOwnProperty.call(message, "entityId"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint64(message.entityId);
            if (message.autoInteract != null && Object.hasOwnProperty.call(message, "autoInteract"))
                writer.uint32(/* id 2, wireType 0 =*/16).bool(message.autoInteract);
            return writer;
        };

        /**
         * Encodes the specified MoveToEntity message, length delimited. Does not implicitly {@link proto.MoveToEntity.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.MoveToEntity
         * @static
         * @param {proto.IMoveToEntity} message MoveToEntity message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        MoveToEntity.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a MoveToEntity message from the specified reader or buffer.
         * @function decode
         * @memberof proto.MoveToEntity
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.MoveToEntity} MoveToEntity
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        MoveToEntity.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.MoveToEntity();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.entityId = reader.uint64();
                        break;
                    }
                case 2: {
                        message.autoInteract = reader.bool();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a MoveToEntity message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.MoveToEntity
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.MoveToEntity} MoveToEntity
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        MoveToEntity.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a MoveToEntity message.
         * @function verify
         * @memberof proto.MoveToEntity
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        MoveToEntity.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.entityId != null && message.hasOwnProperty("entityId"))
                if (!$util.isInteger(message.entityId) && !(message.entityId && $util.isInteger(message.entityId.low) && $util.isInteger(message.entityId.high)))
                    return "entityId: integer|Long expected";
            if (message.autoInteract != null && message.hasOwnProperty("autoInteract"))
                if (typeof message.autoInteract !== "boolean")
                    return "autoInteract: boolean expected";
            return null;
        };

        /**
         * Creates a MoveToEntity message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.MoveToEntity
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.MoveToEntity} MoveToEntity
         */
        MoveToEntity.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.MoveToEntity)
                return object;
            let message = new $root.proto.MoveToEntity();
            if (object.entityId != null)
                if ($util.Long)
                    (message.entityId = $util.Long.fromValue(object.entityId)).unsigned = true;
                else if (typeof object.entityId === "string")
                    message.entityId = parseInt(object.entityId, 10);
                else if (typeof object.entityId === "number")
                    message.entityId = object.entityId;
                else if (typeof object.entityId === "object")
                    message.entityId = new $util.LongBits(object.entityId.low >>> 0, object.entityId.high >>> 0).toNumber(true);
            if (object.autoInteract != null)
                message.autoInteract = Boolean(object.autoInteract);
            return message;
        };

        /**
         * Creates a plain object from a MoveToEntity message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.MoveToEntity
         * @static
         * @param {proto.MoveToEntity} message MoveToEntity
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        MoveToEntity.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                if ($util.Long) {
                    let long = new $util.Long(0, 0, true);
                    object.entityId = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.entityId = options.longs === String ? "0" : 0;
                object.autoInteract = false;
            }
            if (message.entityId != null && message.hasOwnProperty("entityId"))
                if (typeof message.entityId === "number")
                    object.entityId = options.longs === String ? String(message.entityId) : message.entityId;
                else
                    object.entityId = options.longs === String ? $util.Long.prototype.toString.call(message.entityId) : options.longs === Number ? new $util.LongBits(message.entityId.low >>> 0, message.entityId.high >>> 0).toNumber(true) : message.entityId;
            if (message.autoInteract != null && message.hasOwnProperty("autoInteract"))
                object.autoInteract = message.autoInteract;
            return object;
        };

        /**
         * Converts this MoveToEntity to JSON.
         * @function toJSON
         * @memberof proto.MoveToEntity
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        MoveToEntity.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for MoveToEntity
         * @function getTypeUrl
         * @memberof proto.MoveToEntity
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        MoveToEntity.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.MoveToEntity";
        };

        return MoveToEntity;
    })();

    proto.Interact = (function() {

        /**
         * Properties of an Interact.
         * @memberof proto
         * @interface IInteract
         * @property {number|Long|null} [entityId] Interact entityId
         * @property {proto.InteractionType|null} [type] Interact type
         */

        /**
         * Constructs a new Interact.
         * @memberof proto
         * @classdesc Represents an Interact.
         * @implements IInteract
         * @constructor
         * @param {proto.IInteract=} [properties] Properties to set
         */
        function Interact(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * Interact entityId.
         * @member {number|Long} entityId
         * @memberof proto.Interact
         * @instance
         */
        Interact.prototype.entityId = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * Interact type.
         * @member {proto.InteractionType} type
         * @memberof proto.Interact
         * @instance
         */
        Interact.prototype.type = 0;

        /**
         * Creates a new Interact instance using the specified properties.
         * @function create
         * @memberof proto.Interact
         * @static
         * @param {proto.IInteract=} [properties] Properties to set
         * @returns {proto.Interact} Interact instance
         */
        Interact.create = function create(properties) {
            return new Interact(properties);
        };

        /**
         * Encodes the specified Interact message. Does not implicitly {@link proto.Interact.verify|verify} messages.
         * @function encode
         * @memberof proto.Interact
         * @static
         * @param {proto.IInteract} message Interact message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Interact.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.entityId != null && Object.hasOwnProperty.call(message, "entityId"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint64(message.entityId);
            if (message.type != null && Object.hasOwnProperty.call(message, "type"))
                writer.uint32(/* id 2, wireType 0 =*/16).int32(message.type);
            return writer;
        };

        /**
         * Encodes the specified Interact message, length delimited. Does not implicitly {@link proto.Interact.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.Interact
         * @static
         * @param {proto.IInteract} message Interact message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Interact.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes an Interact message from the specified reader or buffer.
         * @function decode
         * @memberof proto.Interact
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.Interact} Interact
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Interact.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.Interact();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.entityId = reader.uint64();
                        break;
                    }
                case 2: {
                        message.type = reader.int32();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes an Interact message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.Interact
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.Interact} Interact
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Interact.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies an Interact message.
         * @function verify
         * @memberof proto.Interact
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        Interact.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.entityId != null && message.hasOwnProperty("entityId"))
                if (!$util.isInteger(message.entityId) && !(message.entityId && $util.isInteger(message.entityId.low) && $util.isInteger(message.entityId.high)))
                    return "entityId: integer|Long expected";
            if (message.type != null && message.hasOwnProperty("type"))
                switch (message.type) {
                default:
                    return "type: enum value expected";
                case 0:
                case 1:
                case 2:
                case 3:
                case 4:
                case 5:
                    break;
                }
            return null;
        };

        /**
         * Creates an Interact message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.Interact
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.Interact} Interact
         */
        Interact.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.Interact)
                return object;
            let message = new $root.proto.Interact();
            if (object.entityId != null)
                if ($util.Long)
                    (message.entityId = $util.Long.fromValue(object.entityId)).unsigned = true;
                else if (typeof object.entityId === "string")
                    message.entityId = parseInt(object.entityId, 10);
                else if (typeof object.entityId === "number")
                    message.entityId = object.entityId;
                else if (typeof object.entityId === "object")
                    message.entityId = new $util.LongBits(object.entityId.low >>> 0, object.entityId.high >>> 0).toNumber(true);
            switch (object.type) {
            default:
                if (typeof object.type === "number") {
                    message.type = object.type;
                    break;
                }
                break;
            case "AUTO":
            case 0:
                message.type = 0;
                break;
            case "GATHER":
            case 1:
                message.type = 1;
                break;
            case "OPEN_CONTAINER":
            case 2:
                message.type = 2;
                break;
            case "CLOSE_CONTAINER":
            case 3:
                message.type = 3;
                break;
            case "USE":
            case 4:
                message.type = 4;
                break;
            case "PICKUP":
            case 5:
                message.type = 5;
                break;
            }
            return message;
        };

        /**
         * Creates a plain object from an Interact message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.Interact
         * @static
         * @param {proto.Interact} message Interact
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        Interact.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                if ($util.Long) {
                    let long = new $util.Long(0, 0, true);
                    object.entityId = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.entityId = options.longs === String ? "0" : 0;
                object.type = options.enums === String ? "AUTO" : 0;
            }
            if (message.entityId != null && message.hasOwnProperty("entityId"))
                if (typeof message.entityId === "number")
                    object.entityId = options.longs === String ? String(message.entityId) : message.entityId;
                else
                    object.entityId = options.longs === String ? $util.Long.prototype.toString.call(message.entityId) : options.longs === Number ? new $util.LongBits(message.entityId.low >>> 0, message.entityId.high >>> 0).toNumber(true) : message.entityId;
            if (message.type != null && message.hasOwnProperty("type"))
                object.type = options.enums === String ? $root.proto.InteractionType[message.type] === undefined ? message.type : $root.proto.InteractionType[message.type] : message.type;
            return object;
        };

        /**
         * Converts this Interact to JSON.
         * @function toJSON
         * @memberof proto.Interact
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        Interact.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for Interact
         * @function getTypeUrl
         * @memberof proto.Interact
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        Interact.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.Interact";
        };

        return Interact;
    })();

    /**
     * InteractionType enum.
     * @name proto.InteractionType
     * @enum {number}
     * @property {number} AUTO=0 AUTO value
     * @property {number} GATHER=1 GATHER value
     * @property {number} OPEN_CONTAINER=2 OPEN_CONTAINER value
     * @property {number} CLOSE_CONTAINER=3 CLOSE_CONTAINER value
     * @property {number} USE=4 USE value
     * @property {number} PICKUP=5 PICKUP value
     */
    proto.InteractionType = (function() {
        const valuesById = {}, values = Object.create(valuesById);
        values[valuesById[0] = "AUTO"] = 0;
        values[valuesById[1] = "GATHER"] = 1;
        values[valuesById[2] = "OPEN_CONTAINER"] = 2;
        values[valuesById[3] = "CLOSE_CONTAINER"] = 3;
        values[valuesById[4] = "USE"] = 4;
        values[valuesById[5] = "PICKUP"] = 5;
        return values;
    })();

    proto.C2S_PlayerAction = (function() {

        /**
         * Properties of a C2S_PlayerAction.
         * @memberof proto
         * @interface IC2S_PlayerAction
         * @property {proto.IMoveTo|null} [moveTo] C2S_PlayerAction moveTo
         * @property {proto.IMoveToEntity|null} [moveToEntity] C2S_PlayerAction moveToEntity
         * @property {proto.IInteract|null} [interact] C2S_PlayerAction interact
         * @property {number|null} [modifiers] C2S_PlayerAction modifiers
         */

        /**
         * Constructs a new C2S_PlayerAction.
         * @memberof proto
         * @classdesc Represents a C2S_PlayerAction.
         * @implements IC2S_PlayerAction
         * @constructor
         * @param {proto.IC2S_PlayerAction=} [properties] Properties to set
         */
        function C2S_PlayerAction(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * C2S_PlayerAction moveTo.
         * @member {proto.IMoveTo|null|undefined} moveTo
         * @memberof proto.C2S_PlayerAction
         * @instance
         */
        C2S_PlayerAction.prototype.moveTo = null;

        /**
         * C2S_PlayerAction moveToEntity.
         * @member {proto.IMoveToEntity|null|undefined} moveToEntity
         * @memberof proto.C2S_PlayerAction
         * @instance
         */
        C2S_PlayerAction.prototype.moveToEntity = null;

        /**
         * C2S_PlayerAction interact.
         * @member {proto.IInteract|null|undefined} interact
         * @memberof proto.C2S_PlayerAction
         * @instance
         */
        C2S_PlayerAction.prototype.interact = null;

        /**
         * C2S_PlayerAction modifiers.
         * @member {number} modifiers
         * @memberof proto.C2S_PlayerAction
         * @instance
         */
        C2S_PlayerAction.prototype.modifiers = 0;

        // OneOf field names bound to virtual getters and setters
        let $oneOfFields;

        /**
         * C2S_PlayerAction action.
         * @member {"moveTo"|"moveToEntity"|"interact"|undefined} action
         * @memberof proto.C2S_PlayerAction
         * @instance
         */
        Object.defineProperty(C2S_PlayerAction.prototype, "action", {
            get: $util.oneOfGetter($oneOfFields = ["moveTo", "moveToEntity", "interact"]),
            set: $util.oneOfSetter($oneOfFields)
        });

        /**
         * Creates a new C2S_PlayerAction instance using the specified properties.
         * @function create
         * @memberof proto.C2S_PlayerAction
         * @static
         * @param {proto.IC2S_PlayerAction=} [properties] Properties to set
         * @returns {proto.C2S_PlayerAction} C2S_PlayerAction instance
         */
        C2S_PlayerAction.create = function create(properties) {
            return new C2S_PlayerAction(properties);
        };

        /**
         * Encodes the specified C2S_PlayerAction message. Does not implicitly {@link proto.C2S_PlayerAction.verify|verify} messages.
         * @function encode
         * @memberof proto.C2S_PlayerAction
         * @static
         * @param {proto.IC2S_PlayerAction} message C2S_PlayerAction message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        C2S_PlayerAction.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.moveTo != null && Object.hasOwnProperty.call(message, "moveTo"))
                $root.proto.MoveTo.encode(message.moveTo, writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
            if (message.moveToEntity != null && Object.hasOwnProperty.call(message, "moveToEntity"))
                $root.proto.MoveToEntity.encode(message.moveToEntity, writer.uint32(/* id 2, wireType 2 =*/18).fork()).ldelim();
            if (message.interact != null && Object.hasOwnProperty.call(message, "interact"))
                $root.proto.Interact.encode(message.interact, writer.uint32(/* id 3, wireType 2 =*/26).fork()).ldelim();
            if (message.modifiers != null && Object.hasOwnProperty.call(message, "modifiers"))
                writer.uint32(/* id 10, wireType 0 =*/80).uint32(message.modifiers);
            return writer;
        };

        /**
         * Encodes the specified C2S_PlayerAction message, length delimited. Does not implicitly {@link proto.C2S_PlayerAction.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.C2S_PlayerAction
         * @static
         * @param {proto.IC2S_PlayerAction} message C2S_PlayerAction message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        C2S_PlayerAction.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a C2S_PlayerAction message from the specified reader or buffer.
         * @function decode
         * @memberof proto.C2S_PlayerAction
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.C2S_PlayerAction} C2S_PlayerAction
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        C2S_PlayerAction.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.C2S_PlayerAction();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.moveTo = $root.proto.MoveTo.decode(reader, reader.uint32());
                        break;
                    }
                case 2: {
                        message.moveToEntity = $root.proto.MoveToEntity.decode(reader, reader.uint32());
                        break;
                    }
                case 3: {
                        message.interact = $root.proto.Interact.decode(reader, reader.uint32());
                        break;
                    }
                case 10: {
                        message.modifiers = reader.uint32();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a C2S_PlayerAction message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.C2S_PlayerAction
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.C2S_PlayerAction} C2S_PlayerAction
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        C2S_PlayerAction.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a C2S_PlayerAction message.
         * @function verify
         * @memberof proto.C2S_PlayerAction
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        C2S_PlayerAction.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            let properties = {};
            if (message.moveTo != null && message.hasOwnProperty("moveTo")) {
                properties.action = 1;
                {
                    let error = $root.proto.MoveTo.verify(message.moveTo);
                    if (error)
                        return "moveTo." + error;
                }
            }
            if (message.moveToEntity != null && message.hasOwnProperty("moveToEntity")) {
                if (properties.action === 1)
                    return "action: multiple values";
                properties.action = 1;
                {
                    let error = $root.proto.MoveToEntity.verify(message.moveToEntity);
                    if (error)
                        return "moveToEntity." + error;
                }
            }
            if (message.interact != null && message.hasOwnProperty("interact")) {
                if (properties.action === 1)
                    return "action: multiple values";
                properties.action = 1;
                {
                    let error = $root.proto.Interact.verify(message.interact);
                    if (error)
                        return "interact." + error;
                }
            }
            if (message.modifiers != null && message.hasOwnProperty("modifiers"))
                if (!$util.isInteger(message.modifiers))
                    return "modifiers: integer expected";
            return null;
        };

        /**
         * Creates a C2S_PlayerAction message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.C2S_PlayerAction
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.C2S_PlayerAction} C2S_PlayerAction
         */
        C2S_PlayerAction.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.C2S_PlayerAction)
                return object;
            let message = new $root.proto.C2S_PlayerAction();
            if (object.moveTo != null) {
                if (typeof object.moveTo !== "object")
                    throw TypeError(".proto.C2S_PlayerAction.moveTo: object expected");
                message.moveTo = $root.proto.MoveTo.fromObject(object.moveTo);
            }
            if (object.moveToEntity != null) {
                if (typeof object.moveToEntity !== "object")
                    throw TypeError(".proto.C2S_PlayerAction.moveToEntity: object expected");
                message.moveToEntity = $root.proto.MoveToEntity.fromObject(object.moveToEntity);
            }
            if (object.interact != null) {
                if (typeof object.interact !== "object")
                    throw TypeError(".proto.C2S_PlayerAction.interact: object expected");
                message.interact = $root.proto.Interact.fromObject(object.interact);
            }
            if (object.modifiers != null)
                message.modifiers = object.modifiers >>> 0;
            return message;
        };

        /**
         * Creates a plain object from a C2S_PlayerAction message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.C2S_PlayerAction
         * @static
         * @param {proto.C2S_PlayerAction} message C2S_PlayerAction
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        C2S_PlayerAction.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults)
                object.modifiers = 0;
            if (message.moveTo != null && message.hasOwnProperty("moveTo")) {
                object.moveTo = $root.proto.MoveTo.toObject(message.moveTo, options);
                if (options.oneofs)
                    object.action = "moveTo";
            }
            if (message.moveToEntity != null && message.hasOwnProperty("moveToEntity")) {
                object.moveToEntity = $root.proto.MoveToEntity.toObject(message.moveToEntity, options);
                if (options.oneofs)
                    object.action = "moveToEntity";
            }
            if (message.interact != null && message.hasOwnProperty("interact")) {
                object.interact = $root.proto.Interact.toObject(message.interact, options);
                if (options.oneofs)
                    object.action = "interact";
            }
            if (message.modifiers != null && message.hasOwnProperty("modifiers"))
                object.modifiers = message.modifiers;
            return object;
        };

        /**
         * Converts this C2S_PlayerAction to JSON.
         * @function toJSON
         * @memberof proto.C2S_PlayerAction
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        C2S_PlayerAction.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for C2S_PlayerAction
         * @function getTypeUrl
         * @memberof proto.C2S_PlayerAction
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        C2S_PlayerAction.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.C2S_PlayerAction";
        };

        return C2S_PlayerAction;
    })();

    proto.C2S_MovementMode = (function() {

        /**
         * Properties of a C2S_MovementMode.
         * @memberof proto
         * @interface IC2S_MovementMode
         * @property {proto.MovementMode|null} [mode] C2S_MovementMode mode
         */

        /**
         * Constructs a new C2S_MovementMode.
         * @memberof proto
         * @classdesc Represents a C2S_MovementMode.
         * @implements IC2S_MovementMode
         * @constructor
         * @param {proto.IC2S_MovementMode=} [properties] Properties to set
         */
        function C2S_MovementMode(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * C2S_MovementMode mode.
         * @member {proto.MovementMode} mode
         * @memberof proto.C2S_MovementMode
         * @instance
         */
        C2S_MovementMode.prototype.mode = 0;

        /**
         * Creates a new C2S_MovementMode instance using the specified properties.
         * @function create
         * @memberof proto.C2S_MovementMode
         * @static
         * @param {proto.IC2S_MovementMode=} [properties] Properties to set
         * @returns {proto.C2S_MovementMode} C2S_MovementMode instance
         */
        C2S_MovementMode.create = function create(properties) {
            return new C2S_MovementMode(properties);
        };

        /**
         * Encodes the specified C2S_MovementMode message. Does not implicitly {@link proto.C2S_MovementMode.verify|verify} messages.
         * @function encode
         * @memberof proto.C2S_MovementMode
         * @static
         * @param {proto.IC2S_MovementMode} message C2S_MovementMode message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        C2S_MovementMode.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.mode != null && Object.hasOwnProperty.call(message, "mode"))
                writer.uint32(/* id 1, wireType 0 =*/8).int32(message.mode);
            return writer;
        };

        /**
         * Encodes the specified C2S_MovementMode message, length delimited. Does not implicitly {@link proto.C2S_MovementMode.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.C2S_MovementMode
         * @static
         * @param {proto.IC2S_MovementMode} message C2S_MovementMode message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        C2S_MovementMode.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a C2S_MovementMode message from the specified reader or buffer.
         * @function decode
         * @memberof proto.C2S_MovementMode
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.C2S_MovementMode} C2S_MovementMode
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        C2S_MovementMode.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.C2S_MovementMode();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.mode = reader.int32();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a C2S_MovementMode message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.C2S_MovementMode
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.C2S_MovementMode} C2S_MovementMode
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        C2S_MovementMode.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a C2S_MovementMode message.
         * @function verify
         * @memberof proto.C2S_MovementMode
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        C2S_MovementMode.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.mode != null && message.hasOwnProperty("mode"))
                switch (message.mode) {
                default:
                    return "mode: enum value expected";
                case 0:
                case 1:
                case 2:
                case 3:
                    break;
                }
            return null;
        };

        /**
         * Creates a C2S_MovementMode message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.C2S_MovementMode
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.C2S_MovementMode} C2S_MovementMode
         */
        C2S_MovementMode.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.C2S_MovementMode)
                return object;
            let message = new $root.proto.C2S_MovementMode();
            switch (object.mode) {
            default:
                if (typeof object.mode === "number") {
                    message.mode = object.mode;
                    break;
                }
                break;
            case "MOVE_MODE_WALK":
            case 0:
                message.mode = 0;
                break;
            case "MOVE_MODE_RUN":
            case 1:
                message.mode = 1;
                break;
            case "MOVE_MODE_FAST_RUN":
            case 2:
                message.mode = 2;
                break;
            case "MOVE_MODE_SWIM":
            case 3:
                message.mode = 3;
                break;
            }
            return message;
        };

        /**
         * Creates a plain object from a C2S_MovementMode message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.C2S_MovementMode
         * @static
         * @param {proto.C2S_MovementMode} message C2S_MovementMode
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        C2S_MovementMode.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults)
                object.mode = options.enums === String ? "MOVE_MODE_WALK" : 0;
            if (message.mode != null && message.hasOwnProperty("mode"))
                object.mode = options.enums === String ? $root.proto.MovementMode[message.mode] === undefined ? message.mode : $root.proto.MovementMode[message.mode] : message.mode;
            return object;
        };

        /**
         * Converts this C2S_MovementMode to JSON.
         * @function toJSON
         * @memberof proto.C2S_MovementMode
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        C2S_MovementMode.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for C2S_MovementMode
         * @function getTypeUrl
         * @memberof proto.C2S_MovementMode
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        C2S_MovementMode.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.C2S_MovementMode";
        };

        return C2S_MovementMode;
    })();

    proto.C2S_Auth = (function() {

        /**
         * Properties of a C2S_Auth.
         * @memberof proto
         * @interface IC2S_Auth
         * @property {string|null} [token] C2S_Auth token
         * @property {string|null} [clientVersion] C2S_Auth clientVersion
         */

        /**
         * Constructs a new C2S_Auth.
         * @memberof proto
         * @classdesc Represents a C2S_Auth.
         * @implements IC2S_Auth
         * @constructor
         * @param {proto.IC2S_Auth=} [properties] Properties to set
         */
        function C2S_Auth(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * C2S_Auth token.
         * @member {string} token
         * @memberof proto.C2S_Auth
         * @instance
         */
        C2S_Auth.prototype.token = "";

        /**
         * C2S_Auth clientVersion.
         * @member {string} clientVersion
         * @memberof proto.C2S_Auth
         * @instance
         */
        C2S_Auth.prototype.clientVersion = "";

        /**
         * Creates a new C2S_Auth instance using the specified properties.
         * @function create
         * @memberof proto.C2S_Auth
         * @static
         * @param {proto.IC2S_Auth=} [properties] Properties to set
         * @returns {proto.C2S_Auth} C2S_Auth instance
         */
        C2S_Auth.create = function create(properties) {
            return new C2S_Auth(properties);
        };

        /**
         * Encodes the specified C2S_Auth message. Does not implicitly {@link proto.C2S_Auth.verify|verify} messages.
         * @function encode
         * @memberof proto.C2S_Auth
         * @static
         * @param {proto.IC2S_Auth} message C2S_Auth message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        C2S_Auth.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.token != null && Object.hasOwnProperty.call(message, "token"))
                writer.uint32(/* id 1, wireType 2 =*/10).string(message.token);
            if (message.clientVersion != null && Object.hasOwnProperty.call(message, "clientVersion"))
                writer.uint32(/* id 2, wireType 2 =*/18).string(message.clientVersion);
            return writer;
        };

        /**
         * Encodes the specified C2S_Auth message, length delimited. Does not implicitly {@link proto.C2S_Auth.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.C2S_Auth
         * @static
         * @param {proto.IC2S_Auth} message C2S_Auth message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        C2S_Auth.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a C2S_Auth message from the specified reader or buffer.
         * @function decode
         * @memberof proto.C2S_Auth
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.C2S_Auth} C2S_Auth
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        C2S_Auth.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.C2S_Auth();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.token = reader.string();
                        break;
                    }
                case 2: {
                        message.clientVersion = reader.string();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a C2S_Auth message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.C2S_Auth
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.C2S_Auth} C2S_Auth
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        C2S_Auth.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a C2S_Auth message.
         * @function verify
         * @memberof proto.C2S_Auth
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        C2S_Auth.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.token != null && message.hasOwnProperty("token"))
                if (!$util.isString(message.token))
                    return "token: string expected";
            if (message.clientVersion != null && message.hasOwnProperty("clientVersion"))
                if (!$util.isString(message.clientVersion))
                    return "clientVersion: string expected";
            return null;
        };

        /**
         * Creates a C2S_Auth message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.C2S_Auth
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.C2S_Auth} C2S_Auth
         */
        C2S_Auth.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.C2S_Auth)
                return object;
            let message = new $root.proto.C2S_Auth();
            if (object.token != null)
                message.token = String(object.token);
            if (object.clientVersion != null)
                message.clientVersion = String(object.clientVersion);
            return message;
        };

        /**
         * Creates a plain object from a C2S_Auth message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.C2S_Auth
         * @static
         * @param {proto.C2S_Auth} message C2S_Auth
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        C2S_Auth.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.token = "";
                object.clientVersion = "";
            }
            if (message.token != null && message.hasOwnProperty("token"))
                object.token = message.token;
            if (message.clientVersion != null && message.hasOwnProperty("clientVersion"))
                object.clientVersion = message.clientVersion;
            return object;
        };

        /**
         * Converts this C2S_Auth to JSON.
         * @function toJSON
         * @memberof proto.C2S_Auth
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        C2S_Auth.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for C2S_Auth
         * @function getTypeUrl
         * @memberof proto.C2S_Auth
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        C2S_Auth.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.C2S_Auth";
        };

        return C2S_Auth;
    })();

    proto.C2S_Ping = (function() {

        /**
         * Properties of a C2S_Ping.
         * @memberof proto
         * @interface IC2S_Ping
         * @property {number|Long|null} [clientTimeMs] C2S_Ping clientTimeMs
         */

        /**
         * Constructs a new C2S_Ping.
         * @memberof proto
         * @classdesc Represents a C2S_Ping.
         * @implements IC2S_Ping
         * @constructor
         * @param {proto.IC2S_Ping=} [properties] Properties to set
         */
        function C2S_Ping(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * C2S_Ping clientTimeMs.
         * @member {number|Long} clientTimeMs
         * @memberof proto.C2S_Ping
         * @instance
         */
        C2S_Ping.prototype.clientTimeMs = $util.Long ? $util.Long.fromBits(0,0,false) : 0;

        /**
         * Creates a new C2S_Ping instance using the specified properties.
         * @function create
         * @memberof proto.C2S_Ping
         * @static
         * @param {proto.IC2S_Ping=} [properties] Properties to set
         * @returns {proto.C2S_Ping} C2S_Ping instance
         */
        C2S_Ping.create = function create(properties) {
            return new C2S_Ping(properties);
        };

        /**
         * Encodes the specified C2S_Ping message. Does not implicitly {@link proto.C2S_Ping.verify|verify} messages.
         * @function encode
         * @memberof proto.C2S_Ping
         * @static
         * @param {proto.IC2S_Ping} message C2S_Ping message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        C2S_Ping.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.clientTimeMs != null && Object.hasOwnProperty.call(message, "clientTimeMs"))
                writer.uint32(/* id 1, wireType 0 =*/8).int64(message.clientTimeMs);
            return writer;
        };

        /**
         * Encodes the specified C2S_Ping message, length delimited. Does not implicitly {@link proto.C2S_Ping.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.C2S_Ping
         * @static
         * @param {proto.IC2S_Ping} message C2S_Ping message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        C2S_Ping.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a C2S_Ping message from the specified reader or buffer.
         * @function decode
         * @memberof proto.C2S_Ping
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.C2S_Ping} C2S_Ping
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        C2S_Ping.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.C2S_Ping();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.clientTimeMs = reader.int64();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a C2S_Ping message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.C2S_Ping
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.C2S_Ping} C2S_Ping
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        C2S_Ping.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a C2S_Ping message.
         * @function verify
         * @memberof proto.C2S_Ping
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        C2S_Ping.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.clientTimeMs != null && message.hasOwnProperty("clientTimeMs"))
                if (!$util.isInteger(message.clientTimeMs) && !(message.clientTimeMs && $util.isInteger(message.clientTimeMs.low) && $util.isInteger(message.clientTimeMs.high)))
                    return "clientTimeMs: integer|Long expected";
            return null;
        };

        /**
         * Creates a C2S_Ping message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.C2S_Ping
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.C2S_Ping} C2S_Ping
         */
        C2S_Ping.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.C2S_Ping)
                return object;
            let message = new $root.proto.C2S_Ping();
            if (object.clientTimeMs != null)
                if ($util.Long)
                    (message.clientTimeMs = $util.Long.fromValue(object.clientTimeMs)).unsigned = false;
                else if (typeof object.clientTimeMs === "string")
                    message.clientTimeMs = parseInt(object.clientTimeMs, 10);
                else if (typeof object.clientTimeMs === "number")
                    message.clientTimeMs = object.clientTimeMs;
                else if (typeof object.clientTimeMs === "object")
                    message.clientTimeMs = new $util.LongBits(object.clientTimeMs.low >>> 0, object.clientTimeMs.high >>> 0).toNumber();
            return message;
        };

        /**
         * Creates a plain object from a C2S_Ping message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.C2S_Ping
         * @static
         * @param {proto.C2S_Ping} message C2S_Ping
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        C2S_Ping.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults)
                if ($util.Long) {
                    let long = new $util.Long(0, 0, false);
                    object.clientTimeMs = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.clientTimeMs = options.longs === String ? "0" : 0;
            if (message.clientTimeMs != null && message.hasOwnProperty("clientTimeMs"))
                if (typeof message.clientTimeMs === "number")
                    object.clientTimeMs = options.longs === String ? String(message.clientTimeMs) : message.clientTimeMs;
                else
                    object.clientTimeMs = options.longs === String ? $util.Long.prototype.toString.call(message.clientTimeMs) : options.longs === Number ? new $util.LongBits(message.clientTimeMs.low >>> 0, message.clientTimeMs.high >>> 0).toNumber() : message.clientTimeMs;
            return object;
        };

        /**
         * Converts this C2S_Ping to JSON.
         * @function toJSON
         * @memberof proto.C2S_Ping
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        C2S_Ping.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for C2S_Ping
         * @function getTypeUrl
         * @memberof proto.C2S_Ping
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        C2S_Ping.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.C2S_Ping";
        };

        return C2S_Ping;
    })();

    proto.ClientMessage = (function() {

        /**
         * Properties of a ClientMessage.
         * @memberof proto
         * @interface IClientMessage
         * @property {number|null} [sequence] ClientMessage sequence
         * @property {proto.IC2S_Auth|null} [auth] ClientMessage auth
         * @property {proto.IC2S_Ping|null} [ping] ClientMessage ping
         * @property {proto.IC2S_PlayerAction|null} [playerAction] ClientMessage playerAction
         * @property {proto.IC2S_MovementMode|null} [movementMode] ClientMessage movementMode
         * @property {proto.IC2S_InventoryOp|null} [inventoryOp] ClientMessage inventoryOp
         */

        /**
         * Constructs a new ClientMessage.
         * @memberof proto
         * @classdesc Represents a ClientMessage.
         * @implements IClientMessage
         * @constructor
         * @param {proto.IClientMessage=} [properties] Properties to set
         */
        function ClientMessage(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * ClientMessage sequence.
         * @member {number} sequence
         * @memberof proto.ClientMessage
         * @instance
         */
        ClientMessage.prototype.sequence = 0;

        /**
         * ClientMessage auth.
         * @member {proto.IC2S_Auth|null|undefined} auth
         * @memberof proto.ClientMessage
         * @instance
         */
        ClientMessage.prototype.auth = null;

        /**
         * ClientMessage ping.
         * @member {proto.IC2S_Ping|null|undefined} ping
         * @memberof proto.ClientMessage
         * @instance
         */
        ClientMessage.prototype.ping = null;

        /**
         * ClientMessage playerAction.
         * @member {proto.IC2S_PlayerAction|null|undefined} playerAction
         * @memberof proto.ClientMessage
         * @instance
         */
        ClientMessage.prototype.playerAction = null;

        /**
         * ClientMessage movementMode.
         * @member {proto.IC2S_MovementMode|null|undefined} movementMode
         * @memberof proto.ClientMessage
         * @instance
         */
        ClientMessage.prototype.movementMode = null;

        /**
         * ClientMessage inventoryOp.
         * @member {proto.IC2S_InventoryOp|null|undefined} inventoryOp
         * @memberof proto.ClientMessage
         * @instance
         */
        ClientMessage.prototype.inventoryOp = null;

        // OneOf field names bound to virtual getters and setters
        let $oneOfFields;

        /**
         * ClientMessage payload.
         * @member {"auth"|"ping"|"playerAction"|"movementMode"|"inventoryOp"|undefined} payload
         * @memberof proto.ClientMessage
         * @instance
         */
        Object.defineProperty(ClientMessage.prototype, "payload", {
            get: $util.oneOfGetter($oneOfFields = ["auth", "ping", "playerAction", "movementMode", "inventoryOp"]),
            set: $util.oneOfSetter($oneOfFields)
        });

        /**
         * Creates a new ClientMessage instance using the specified properties.
         * @function create
         * @memberof proto.ClientMessage
         * @static
         * @param {proto.IClientMessage=} [properties] Properties to set
         * @returns {proto.ClientMessage} ClientMessage instance
         */
        ClientMessage.create = function create(properties) {
            return new ClientMessage(properties);
        };

        /**
         * Encodes the specified ClientMessage message. Does not implicitly {@link proto.ClientMessage.verify|verify} messages.
         * @function encode
         * @memberof proto.ClientMessage
         * @static
         * @param {proto.IClientMessage} message ClientMessage message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        ClientMessage.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.sequence != null && Object.hasOwnProperty.call(message, "sequence"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint32(message.sequence);
            if (message.auth != null && Object.hasOwnProperty.call(message, "auth"))
                $root.proto.C2S_Auth.encode(message.auth, writer.uint32(/* id 10, wireType 2 =*/82).fork()).ldelim();
            if (message.ping != null && Object.hasOwnProperty.call(message, "ping"))
                $root.proto.C2S_Ping.encode(message.ping, writer.uint32(/* id 11, wireType 2 =*/90).fork()).ldelim();
            if (message.playerAction != null && Object.hasOwnProperty.call(message, "playerAction"))
                $root.proto.C2S_PlayerAction.encode(message.playerAction, writer.uint32(/* id 12, wireType 2 =*/98).fork()).ldelim();
            if (message.movementMode != null && Object.hasOwnProperty.call(message, "movementMode"))
                $root.proto.C2S_MovementMode.encode(message.movementMode, writer.uint32(/* id 13, wireType 2 =*/106).fork()).ldelim();
            if (message.inventoryOp != null && Object.hasOwnProperty.call(message, "inventoryOp"))
                $root.proto.C2S_InventoryOp.encode(message.inventoryOp, writer.uint32(/* id 14, wireType 2 =*/114).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified ClientMessage message, length delimited. Does not implicitly {@link proto.ClientMessage.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.ClientMessage
         * @static
         * @param {proto.IClientMessage} message ClientMessage message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        ClientMessage.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a ClientMessage message from the specified reader or buffer.
         * @function decode
         * @memberof proto.ClientMessage
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.ClientMessage} ClientMessage
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        ClientMessage.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.ClientMessage();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.sequence = reader.uint32();
                        break;
                    }
                case 10: {
                        message.auth = $root.proto.C2S_Auth.decode(reader, reader.uint32());
                        break;
                    }
                case 11: {
                        message.ping = $root.proto.C2S_Ping.decode(reader, reader.uint32());
                        break;
                    }
                case 12: {
                        message.playerAction = $root.proto.C2S_PlayerAction.decode(reader, reader.uint32());
                        break;
                    }
                case 13: {
                        message.movementMode = $root.proto.C2S_MovementMode.decode(reader, reader.uint32());
                        break;
                    }
                case 14: {
                        message.inventoryOp = $root.proto.C2S_InventoryOp.decode(reader, reader.uint32());
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a ClientMessage message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.ClientMessage
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.ClientMessage} ClientMessage
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        ClientMessage.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a ClientMessage message.
         * @function verify
         * @memberof proto.ClientMessage
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        ClientMessage.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            let properties = {};
            if (message.sequence != null && message.hasOwnProperty("sequence"))
                if (!$util.isInteger(message.sequence))
                    return "sequence: integer expected";
            if (message.auth != null && message.hasOwnProperty("auth")) {
                properties.payload = 1;
                {
                    let error = $root.proto.C2S_Auth.verify(message.auth);
                    if (error)
                        return "auth." + error;
                }
            }
            if (message.ping != null && message.hasOwnProperty("ping")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.proto.C2S_Ping.verify(message.ping);
                    if (error)
                        return "ping." + error;
                }
            }
            if (message.playerAction != null && message.hasOwnProperty("playerAction")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.proto.C2S_PlayerAction.verify(message.playerAction);
                    if (error)
                        return "playerAction." + error;
                }
            }
            if (message.movementMode != null && message.hasOwnProperty("movementMode")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.proto.C2S_MovementMode.verify(message.movementMode);
                    if (error)
                        return "movementMode." + error;
                }
            }
            if (message.inventoryOp != null && message.hasOwnProperty("inventoryOp")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.proto.C2S_InventoryOp.verify(message.inventoryOp);
                    if (error)
                        return "inventoryOp." + error;
                }
            }
            return null;
        };

        /**
         * Creates a ClientMessage message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.ClientMessage
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.ClientMessage} ClientMessage
         */
        ClientMessage.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.ClientMessage)
                return object;
            let message = new $root.proto.ClientMessage();
            if (object.sequence != null)
                message.sequence = object.sequence >>> 0;
            if (object.auth != null) {
                if (typeof object.auth !== "object")
                    throw TypeError(".proto.ClientMessage.auth: object expected");
                message.auth = $root.proto.C2S_Auth.fromObject(object.auth);
            }
            if (object.ping != null) {
                if (typeof object.ping !== "object")
                    throw TypeError(".proto.ClientMessage.ping: object expected");
                message.ping = $root.proto.C2S_Ping.fromObject(object.ping);
            }
            if (object.playerAction != null) {
                if (typeof object.playerAction !== "object")
                    throw TypeError(".proto.ClientMessage.playerAction: object expected");
                message.playerAction = $root.proto.C2S_PlayerAction.fromObject(object.playerAction);
            }
            if (object.movementMode != null) {
                if (typeof object.movementMode !== "object")
                    throw TypeError(".proto.ClientMessage.movementMode: object expected");
                message.movementMode = $root.proto.C2S_MovementMode.fromObject(object.movementMode);
            }
            if (object.inventoryOp != null) {
                if (typeof object.inventoryOp !== "object")
                    throw TypeError(".proto.ClientMessage.inventoryOp: object expected");
                message.inventoryOp = $root.proto.C2S_InventoryOp.fromObject(object.inventoryOp);
            }
            return message;
        };

        /**
         * Creates a plain object from a ClientMessage message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.ClientMessage
         * @static
         * @param {proto.ClientMessage} message ClientMessage
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        ClientMessage.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults)
                object.sequence = 0;
            if (message.sequence != null && message.hasOwnProperty("sequence"))
                object.sequence = message.sequence;
            if (message.auth != null && message.hasOwnProperty("auth")) {
                object.auth = $root.proto.C2S_Auth.toObject(message.auth, options);
                if (options.oneofs)
                    object.payload = "auth";
            }
            if (message.ping != null && message.hasOwnProperty("ping")) {
                object.ping = $root.proto.C2S_Ping.toObject(message.ping, options);
                if (options.oneofs)
                    object.payload = "ping";
            }
            if (message.playerAction != null && message.hasOwnProperty("playerAction")) {
                object.playerAction = $root.proto.C2S_PlayerAction.toObject(message.playerAction, options);
                if (options.oneofs)
                    object.payload = "playerAction";
            }
            if (message.movementMode != null && message.hasOwnProperty("movementMode")) {
                object.movementMode = $root.proto.C2S_MovementMode.toObject(message.movementMode, options);
                if (options.oneofs)
                    object.payload = "movementMode";
            }
            if (message.inventoryOp != null && message.hasOwnProperty("inventoryOp")) {
                object.inventoryOp = $root.proto.C2S_InventoryOp.toObject(message.inventoryOp, options);
                if (options.oneofs)
                    object.payload = "inventoryOp";
            }
            return object;
        };

        /**
         * Converts this ClientMessage to JSON.
         * @function toJSON
         * @memberof proto.ClientMessage
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        ClientMessage.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for ClientMessage
         * @function getTypeUrl
         * @memberof proto.ClientMessage
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        ClientMessage.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.ClientMessage";
        };

        return ClientMessage;
    })();

    proto.S2C_AuthResult = (function() {

        /**
         * Properties of a S2C_AuthResult.
         * @memberof proto
         * @interface IS2C_AuthResult
         * @property {boolean|null} [success] S2C_AuthResult success
         * @property {string|null} [errorMessage] S2C_AuthResult errorMessage
         */

        /**
         * Constructs a new S2C_AuthResult.
         * @memberof proto
         * @classdesc Represents a S2C_AuthResult.
         * @implements IS2C_AuthResult
         * @constructor
         * @param {proto.IS2C_AuthResult=} [properties] Properties to set
         */
        function S2C_AuthResult(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * S2C_AuthResult success.
         * @member {boolean} success
         * @memberof proto.S2C_AuthResult
         * @instance
         */
        S2C_AuthResult.prototype.success = false;

        /**
         * S2C_AuthResult errorMessage.
         * @member {string} errorMessage
         * @memberof proto.S2C_AuthResult
         * @instance
         */
        S2C_AuthResult.prototype.errorMessage = "";

        /**
         * Creates a new S2C_AuthResult instance using the specified properties.
         * @function create
         * @memberof proto.S2C_AuthResult
         * @static
         * @param {proto.IS2C_AuthResult=} [properties] Properties to set
         * @returns {proto.S2C_AuthResult} S2C_AuthResult instance
         */
        S2C_AuthResult.create = function create(properties) {
            return new S2C_AuthResult(properties);
        };

        /**
         * Encodes the specified S2C_AuthResult message. Does not implicitly {@link proto.S2C_AuthResult.verify|verify} messages.
         * @function encode
         * @memberof proto.S2C_AuthResult
         * @static
         * @param {proto.IS2C_AuthResult} message S2C_AuthResult message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_AuthResult.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.success != null && Object.hasOwnProperty.call(message, "success"))
                writer.uint32(/* id 1, wireType 0 =*/8).bool(message.success);
            if (message.errorMessage != null && Object.hasOwnProperty.call(message, "errorMessage"))
                writer.uint32(/* id 2, wireType 2 =*/18).string(message.errorMessage);
            return writer;
        };

        /**
         * Encodes the specified S2C_AuthResult message, length delimited. Does not implicitly {@link proto.S2C_AuthResult.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.S2C_AuthResult
         * @static
         * @param {proto.IS2C_AuthResult} message S2C_AuthResult message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_AuthResult.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a S2C_AuthResult message from the specified reader or buffer.
         * @function decode
         * @memberof proto.S2C_AuthResult
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.S2C_AuthResult} S2C_AuthResult
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_AuthResult.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.S2C_AuthResult();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.success = reader.bool();
                        break;
                    }
                case 2: {
                        message.errorMessage = reader.string();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a S2C_AuthResult message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.S2C_AuthResult
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.S2C_AuthResult} S2C_AuthResult
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_AuthResult.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a S2C_AuthResult message.
         * @function verify
         * @memberof proto.S2C_AuthResult
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        S2C_AuthResult.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.success != null && message.hasOwnProperty("success"))
                if (typeof message.success !== "boolean")
                    return "success: boolean expected";
            if (message.errorMessage != null && message.hasOwnProperty("errorMessage"))
                if (!$util.isString(message.errorMessage))
                    return "errorMessage: string expected";
            return null;
        };

        /**
         * Creates a S2C_AuthResult message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.S2C_AuthResult
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.S2C_AuthResult} S2C_AuthResult
         */
        S2C_AuthResult.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.S2C_AuthResult)
                return object;
            let message = new $root.proto.S2C_AuthResult();
            if (object.success != null)
                message.success = Boolean(object.success);
            if (object.errorMessage != null)
                message.errorMessage = String(object.errorMessage);
            return message;
        };

        /**
         * Creates a plain object from a S2C_AuthResult message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.S2C_AuthResult
         * @static
         * @param {proto.S2C_AuthResult} message S2C_AuthResult
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        S2C_AuthResult.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.success = false;
                object.errorMessage = "";
            }
            if (message.success != null && message.hasOwnProperty("success"))
                object.success = message.success;
            if (message.errorMessage != null && message.hasOwnProperty("errorMessage"))
                object.errorMessage = message.errorMessage;
            return object;
        };

        /**
         * Converts this S2C_AuthResult to JSON.
         * @function toJSON
         * @memberof proto.S2C_AuthResult
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        S2C_AuthResult.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for S2C_AuthResult
         * @function getTypeUrl
         * @memberof proto.S2C_AuthResult
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        S2C_AuthResult.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.S2C_AuthResult";
        };

        return S2C_AuthResult;
    })();

    proto.S2C_Pong = (function() {

        /**
         * Properties of a S2C_Pong.
         * @memberof proto
         * @interface IS2C_Pong
         * @property {number|Long|null} [clientTimeMs] S2C_Pong clientTimeMs
         * @property {number|Long|null} [serverTimeMs] S2C_Pong serverTimeMs
         */

        /**
         * Constructs a new S2C_Pong.
         * @memberof proto
         * @classdesc Represents a S2C_Pong.
         * @implements IS2C_Pong
         * @constructor
         * @param {proto.IS2C_Pong=} [properties] Properties to set
         */
        function S2C_Pong(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * S2C_Pong clientTimeMs.
         * @member {number|Long} clientTimeMs
         * @memberof proto.S2C_Pong
         * @instance
         */
        S2C_Pong.prototype.clientTimeMs = $util.Long ? $util.Long.fromBits(0,0,false) : 0;

        /**
         * S2C_Pong serverTimeMs.
         * @member {number|Long} serverTimeMs
         * @memberof proto.S2C_Pong
         * @instance
         */
        S2C_Pong.prototype.serverTimeMs = $util.Long ? $util.Long.fromBits(0,0,false) : 0;

        /**
         * Creates a new S2C_Pong instance using the specified properties.
         * @function create
         * @memberof proto.S2C_Pong
         * @static
         * @param {proto.IS2C_Pong=} [properties] Properties to set
         * @returns {proto.S2C_Pong} S2C_Pong instance
         */
        S2C_Pong.create = function create(properties) {
            return new S2C_Pong(properties);
        };

        /**
         * Encodes the specified S2C_Pong message. Does not implicitly {@link proto.S2C_Pong.verify|verify} messages.
         * @function encode
         * @memberof proto.S2C_Pong
         * @static
         * @param {proto.IS2C_Pong} message S2C_Pong message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_Pong.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.clientTimeMs != null && Object.hasOwnProperty.call(message, "clientTimeMs"))
                writer.uint32(/* id 1, wireType 0 =*/8).int64(message.clientTimeMs);
            if (message.serverTimeMs != null && Object.hasOwnProperty.call(message, "serverTimeMs"))
                writer.uint32(/* id 2, wireType 0 =*/16).int64(message.serverTimeMs);
            return writer;
        };

        /**
         * Encodes the specified S2C_Pong message, length delimited. Does not implicitly {@link proto.S2C_Pong.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.S2C_Pong
         * @static
         * @param {proto.IS2C_Pong} message S2C_Pong message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_Pong.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a S2C_Pong message from the specified reader or buffer.
         * @function decode
         * @memberof proto.S2C_Pong
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.S2C_Pong} S2C_Pong
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_Pong.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.S2C_Pong();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.clientTimeMs = reader.int64();
                        break;
                    }
                case 2: {
                        message.serverTimeMs = reader.int64();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a S2C_Pong message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.S2C_Pong
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.S2C_Pong} S2C_Pong
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_Pong.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a S2C_Pong message.
         * @function verify
         * @memberof proto.S2C_Pong
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        S2C_Pong.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.clientTimeMs != null && message.hasOwnProperty("clientTimeMs"))
                if (!$util.isInteger(message.clientTimeMs) && !(message.clientTimeMs && $util.isInteger(message.clientTimeMs.low) && $util.isInteger(message.clientTimeMs.high)))
                    return "clientTimeMs: integer|Long expected";
            if (message.serverTimeMs != null && message.hasOwnProperty("serverTimeMs"))
                if (!$util.isInteger(message.serverTimeMs) && !(message.serverTimeMs && $util.isInteger(message.serverTimeMs.low) && $util.isInteger(message.serverTimeMs.high)))
                    return "serverTimeMs: integer|Long expected";
            return null;
        };

        /**
         * Creates a S2C_Pong message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.S2C_Pong
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.S2C_Pong} S2C_Pong
         */
        S2C_Pong.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.S2C_Pong)
                return object;
            let message = new $root.proto.S2C_Pong();
            if (object.clientTimeMs != null)
                if ($util.Long)
                    (message.clientTimeMs = $util.Long.fromValue(object.clientTimeMs)).unsigned = false;
                else if (typeof object.clientTimeMs === "string")
                    message.clientTimeMs = parseInt(object.clientTimeMs, 10);
                else if (typeof object.clientTimeMs === "number")
                    message.clientTimeMs = object.clientTimeMs;
                else if (typeof object.clientTimeMs === "object")
                    message.clientTimeMs = new $util.LongBits(object.clientTimeMs.low >>> 0, object.clientTimeMs.high >>> 0).toNumber();
            if (object.serverTimeMs != null)
                if ($util.Long)
                    (message.serverTimeMs = $util.Long.fromValue(object.serverTimeMs)).unsigned = false;
                else if (typeof object.serverTimeMs === "string")
                    message.serverTimeMs = parseInt(object.serverTimeMs, 10);
                else if (typeof object.serverTimeMs === "number")
                    message.serverTimeMs = object.serverTimeMs;
                else if (typeof object.serverTimeMs === "object")
                    message.serverTimeMs = new $util.LongBits(object.serverTimeMs.low >>> 0, object.serverTimeMs.high >>> 0).toNumber();
            return message;
        };

        /**
         * Creates a plain object from a S2C_Pong message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.S2C_Pong
         * @static
         * @param {proto.S2C_Pong} message S2C_Pong
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        S2C_Pong.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                if ($util.Long) {
                    let long = new $util.Long(0, 0, false);
                    object.clientTimeMs = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.clientTimeMs = options.longs === String ? "0" : 0;
                if ($util.Long) {
                    let long = new $util.Long(0, 0, false);
                    object.serverTimeMs = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.serverTimeMs = options.longs === String ? "0" : 0;
            }
            if (message.clientTimeMs != null && message.hasOwnProperty("clientTimeMs"))
                if (typeof message.clientTimeMs === "number")
                    object.clientTimeMs = options.longs === String ? String(message.clientTimeMs) : message.clientTimeMs;
                else
                    object.clientTimeMs = options.longs === String ? $util.Long.prototype.toString.call(message.clientTimeMs) : options.longs === Number ? new $util.LongBits(message.clientTimeMs.low >>> 0, message.clientTimeMs.high >>> 0).toNumber() : message.clientTimeMs;
            if (message.serverTimeMs != null && message.hasOwnProperty("serverTimeMs"))
                if (typeof message.serverTimeMs === "number")
                    object.serverTimeMs = options.longs === String ? String(message.serverTimeMs) : message.serverTimeMs;
                else
                    object.serverTimeMs = options.longs === String ? $util.Long.prototype.toString.call(message.serverTimeMs) : options.longs === Number ? new $util.LongBits(message.serverTimeMs.low >>> 0, message.serverTimeMs.high >>> 0).toNumber() : message.serverTimeMs;
            return object;
        };

        /**
         * Converts this S2C_Pong to JSON.
         * @function toJSON
         * @memberof proto.S2C_Pong
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        S2C_Pong.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for S2C_Pong
         * @function getTypeUrl
         * @memberof proto.S2C_Pong
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        S2C_Pong.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.S2C_Pong";
        };

        return S2C_Pong;
    })();

    proto.S2C_PlayerEnterWorld = (function() {

        /**
         * Properties of a S2C_PlayerEnterWorld.
         * @memberof proto
         * @interface IS2C_PlayerEnterWorld
         * @property {number|Long|null} [entityId] S2C_PlayerEnterWorld entityId
         * @property {string|null} [name] S2C_PlayerEnterWorld name
         * @property {number|null} [coordPerTile] S2C_PlayerEnterWorld coordPerTile
         * @property {number|null} [chunkSize] S2C_PlayerEnterWorld chunkSize
         * @property {number|null} [streamEpoch] S2C_PlayerEnterWorld streamEpoch
         */

        /**
         * Constructs a new S2C_PlayerEnterWorld.
         * @memberof proto
         * @classdesc Represents a S2C_PlayerEnterWorld.
         * @implements IS2C_PlayerEnterWorld
         * @constructor
         * @param {proto.IS2C_PlayerEnterWorld=} [properties] Properties to set
         */
        function S2C_PlayerEnterWorld(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * S2C_PlayerEnterWorld entityId.
         * @member {number|Long} entityId
         * @memberof proto.S2C_PlayerEnterWorld
         * @instance
         */
        S2C_PlayerEnterWorld.prototype.entityId = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * S2C_PlayerEnterWorld name.
         * @member {string} name
         * @memberof proto.S2C_PlayerEnterWorld
         * @instance
         */
        S2C_PlayerEnterWorld.prototype.name = "";

        /**
         * S2C_PlayerEnterWorld coordPerTile.
         * @member {number} coordPerTile
         * @memberof proto.S2C_PlayerEnterWorld
         * @instance
         */
        S2C_PlayerEnterWorld.prototype.coordPerTile = 0;

        /**
         * S2C_PlayerEnterWorld chunkSize.
         * @member {number} chunkSize
         * @memberof proto.S2C_PlayerEnterWorld
         * @instance
         */
        S2C_PlayerEnterWorld.prototype.chunkSize = 0;

        /**
         * S2C_PlayerEnterWorld streamEpoch.
         * @member {number} streamEpoch
         * @memberof proto.S2C_PlayerEnterWorld
         * @instance
         */
        S2C_PlayerEnterWorld.prototype.streamEpoch = 0;

        /**
         * Creates a new S2C_PlayerEnterWorld instance using the specified properties.
         * @function create
         * @memberof proto.S2C_PlayerEnterWorld
         * @static
         * @param {proto.IS2C_PlayerEnterWorld=} [properties] Properties to set
         * @returns {proto.S2C_PlayerEnterWorld} S2C_PlayerEnterWorld instance
         */
        S2C_PlayerEnterWorld.create = function create(properties) {
            return new S2C_PlayerEnterWorld(properties);
        };

        /**
         * Encodes the specified S2C_PlayerEnterWorld message. Does not implicitly {@link proto.S2C_PlayerEnterWorld.verify|verify} messages.
         * @function encode
         * @memberof proto.S2C_PlayerEnterWorld
         * @static
         * @param {proto.IS2C_PlayerEnterWorld} message S2C_PlayerEnterWorld message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_PlayerEnterWorld.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.entityId != null && Object.hasOwnProperty.call(message, "entityId"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint64(message.entityId);
            if (message.name != null && Object.hasOwnProperty.call(message, "name"))
                writer.uint32(/* id 2, wireType 2 =*/18).string(message.name);
            if (message.coordPerTile != null && Object.hasOwnProperty.call(message, "coordPerTile"))
                writer.uint32(/* id 3, wireType 0 =*/24).uint32(message.coordPerTile);
            if (message.chunkSize != null && Object.hasOwnProperty.call(message, "chunkSize"))
                writer.uint32(/* id 4, wireType 0 =*/32).uint32(message.chunkSize);
            if (message.streamEpoch != null && Object.hasOwnProperty.call(message, "streamEpoch"))
                writer.uint32(/* id 9, wireType 0 =*/72).uint32(message.streamEpoch);
            return writer;
        };

        /**
         * Encodes the specified S2C_PlayerEnterWorld message, length delimited. Does not implicitly {@link proto.S2C_PlayerEnterWorld.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.S2C_PlayerEnterWorld
         * @static
         * @param {proto.IS2C_PlayerEnterWorld} message S2C_PlayerEnterWorld message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_PlayerEnterWorld.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a S2C_PlayerEnterWorld message from the specified reader or buffer.
         * @function decode
         * @memberof proto.S2C_PlayerEnterWorld
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.S2C_PlayerEnterWorld} S2C_PlayerEnterWorld
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_PlayerEnterWorld.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.S2C_PlayerEnterWorld();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.entityId = reader.uint64();
                        break;
                    }
                case 2: {
                        message.name = reader.string();
                        break;
                    }
                case 3: {
                        message.coordPerTile = reader.uint32();
                        break;
                    }
                case 4: {
                        message.chunkSize = reader.uint32();
                        break;
                    }
                case 9: {
                        message.streamEpoch = reader.uint32();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a S2C_PlayerEnterWorld message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.S2C_PlayerEnterWorld
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.S2C_PlayerEnterWorld} S2C_PlayerEnterWorld
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_PlayerEnterWorld.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a S2C_PlayerEnterWorld message.
         * @function verify
         * @memberof proto.S2C_PlayerEnterWorld
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        S2C_PlayerEnterWorld.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.entityId != null && message.hasOwnProperty("entityId"))
                if (!$util.isInteger(message.entityId) && !(message.entityId && $util.isInteger(message.entityId.low) && $util.isInteger(message.entityId.high)))
                    return "entityId: integer|Long expected";
            if (message.name != null && message.hasOwnProperty("name"))
                if (!$util.isString(message.name))
                    return "name: string expected";
            if (message.coordPerTile != null && message.hasOwnProperty("coordPerTile"))
                if (!$util.isInteger(message.coordPerTile))
                    return "coordPerTile: integer expected";
            if (message.chunkSize != null && message.hasOwnProperty("chunkSize"))
                if (!$util.isInteger(message.chunkSize))
                    return "chunkSize: integer expected";
            if (message.streamEpoch != null && message.hasOwnProperty("streamEpoch"))
                if (!$util.isInteger(message.streamEpoch))
                    return "streamEpoch: integer expected";
            return null;
        };

        /**
         * Creates a S2C_PlayerEnterWorld message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.S2C_PlayerEnterWorld
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.S2C_PlayerEnterWorld} S2C_PlayerEnterWorld
         */
        S2C_PlayerEnterWorld.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.S2C_PlayerEnterWorld)
                return object;
            let message = new $root.proto.S2C_PlayerEnterWorld();
            if (object.entityId != null)
                if ($util.Long)
                    (message.entityId = $util.Long.fromValue(object.entityId)).unsigned = true;
                else if (typeof object.entityId === "string")
                    message.entityId = parseInt(object.entityId, 10);
                else if (typeof object.entityId === "number")
                    message.entityId = object.entityId;
                else if (typeof object.entityId === "object")
                    message.entityId = new $util.LongBits(object.entityId.low >>> 0, object.entityId.high >>> 0).toNumber(true);
            if (object.name != null)
                message.name = String(object.name);
            if (object.coordPerTile != null)
                message.coordPerTile = object.coordPerTile >>> 0;
            if (object.chunkSize != null)
                message.chunkSize = object.chunkSize >>> 0;
            if (object.streamEpoch != null)
                message.streamEpoch = object.streamEpoch >>> 0;
            return message;
        };

        /**
         * Creates a plain object from a S2C_PlayerEnterWorld message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.S2C_PlayerEnterWorld
         * @static
         * @param {proto.S2C_PlayerEnterWorld} message S2C_PlayerEnterWorld
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        S2C_PlayerEnterWorld.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                if ($util.Long) {
                    let long = new $util.Long(0, 0, true);
                    object.entityId = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.entityId = options.longs === String ? "0" : 0;
                object.name = "";
                object.coordPerTile = 0;
                object.chunkSize = 0;
                object.streamEpoch = 0;
            }
            if (message.entityId != null && message.hasOwnProperty("entityId"))
                if (typeof message.entityId === "number")
                    object.entityId = options.longs === String ? String(message.entityId) : message.entityId;
                else
                    object.entityId = options.longs === String ? $util.Long.prototype.toString.call(message.entityId) : options.longs === Number ? new $util.LongBits(message.entityId.low >>> 0, message.entityId.high >>> 0).toNumber(true) : message.entityId;
            if (message.name != null && message.hasOwnProperty("name"))
                object.name = message.name;
            if (message.coordPerTile != null && message.hasOwnProperty("coordPerTile"))
                object.coordPerTile = message.coordPerTile;
            if (message.chunkSize != null && message.hasOwnProperty("chunkSize"))
                object.chunkSize = message.chunkSize;
            if (message.streamEpoch != null && message.hasOwnProperty("streamEpoch"))
                object.streamEpoch = message.streamEpoch;
            return object;
        };

        /**
         * Converts this S2C_PlayerEnterWorld to JSON.
         * @function toJSON
         * @memberof proto.S2C_PlayerEnterWorld
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        S2C_PlayerEnterWorld.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for S2C_PlayerEnterWorld
         * @function getTypeUrl
         * @memberof proto.S2C_PlayerEnterWorld
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        S2C_PlayerEnterWorld.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.S2C_PlayerEnterWorld";
        };

        return S2C_PlayerEnterWorld;
    })();

    proto.S2C_PlayerLeaveWorld = (function() {

        /**
         * Properties of a S2C_PlayerLeaveWorld.
         * @memberof proto
         * @interface IS2C_PlayerLeaveWorld
         * @property {number|Long|null} [entityId] S2C_PlayerLeaveWorld entityId
         */

        /**
         * Constructs a new S2C_PlayerLeaveWorld.
         * @memberof proto
         * @classdesc Represents a S2C_PlayerLeaveWorld.
         * @implements IS2C_PlayerLeaveWorld
         * @constructor
         * @param {proto.IS2C_PlayerLeaveWorld=} [properties] Properties to set
         */
        function S2C_PlayerLeaveWorld(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * S2C_PlayerLeaveWorld entityId.
         * @member {number|Long} entityId
         * @memberof proto.S2C_PlayerLeaveWorld
         * @instance
         */
        S2C_PlayerLeaveWorld.prototype.entityId = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * Creates a new S2C_PlayerLeaveWorld instance using the specified properties.
         * @function create
         * @memberof proto.S2C_PlayerLeaveWorld
         * @static
         * @param {proto.IS2C_PlayerLeaveWorld=} [properties] Properties to set
         * @returns {proto.S2C_PlayerLeaveWorld} S2C_PlayerLeaveWorld instance
         */
        S2C_PlayerLeaveWorld.create = function create(properties) {
            return new S2C_PlayerLeaveWorld(properties);
        };

        /**
         * Encodes the specified S2C_PlayerLeaveWorld message. Does not implicitly {@link proto.S2C_PlayerLeaveWorld.verify|verify} messages.
         * @function encode
         * @memberof proto.S2C_PlayerLeaveWorld
         * @static
         * @param {proto.IS2C_PlayerLeaveWorld} message S2C_PlayerLeaveWorld message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_PlayerLeaveWorld.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.entityId != null && Object.hasOwnProperty.call(message, "entityId"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint64(message.entityId);
            return writer;
        };

        /**
         * Encodes the specified S2C_PlayerLeaveWorld message, length delimited. Does not implicitly {@link proto.S2C_PlayerLeaveWorld.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.S2C_PlayerLeaveWorld
         * @static
         * @param {proto.IS2C_PlayerLeaveWorld} message S2C_PlayerLeaveWorld message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_PlayerLeaveWorld.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a S2C_PlayerLeaveWorld message from the specified reader or buffer.
         * @function decode
         * @memberof proto.S2C_PlayerLeaveWorld
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.S2C_PlayerLeaveWorld} S2C_PlayerLeaveWorld
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_PlayerLeaveWorld.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.S2C_PlayerLeaveWorld();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.entityId = reader.uint64();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a S2C_PlayerLeaveWorld message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.S2C_PlayerLeaveWorld
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.S2C_PlayerLeaveWorld} S2C_PlayerLeaveWorld
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_PlayerLeaveWorld.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a S2C_PlayerLeaveWorld message.
         * @function verify
         * @memberof proto.S2C_PlayerLeaveWorld
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        S2C_PlayerLeaveWorld.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.entityId != null && message.hasOwnProperty("entityId"))
                if (!$util.isInteger(message.entityId) && !(message.entityId && $util.isInteger(message.entityId.low) && $util.isInteger(message.entityId.high)))
                    return "entityId: integer|Long expected";
            return null;
        };

        /**
         * Creates a S2C_PlayerLeaveWorld message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.S2C_PlayerLeaveWorld
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.S2C_PlayerLeaveWorld} S2C_PlayerLeaveWorld
         */
        S2C_PlayerLeaveWorld.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.S2C_PlayerLeaveWorld)
                return object;
            let message = new $root.proto.S2C_PlayerLeaveWorld();
            if (object.entityId != null)
                if ($util.Long)
                    (message.entityId = $util.Long.fromValue(object.entityId)).unsigned = true;
                else if (typeof object.entityId === "string")
                    message.entityId = parseInt(object.entityId, 10);
                else if (typeof object.entityId === "number")
                    message.entityId = object.entityId;
                else if (typeof object.entityId === "object")
                    message.entityId = new $util.LongBits(object.entityId.low >>> 0, object.entityId.high >>> 0).toNumber(true);
            return message;
        };

        /**
         * Creates a plain object from a S2C_PlayerLeaveWorld message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.S2C_PlayerLeaveWorld
         * @static
         * @param {proto.S2C_PlayerLeaveWorld} message S2C_PlayerLeaveWorld
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        S2C_PlayerLeaveWorld.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults)
                if ($util.Long) {
                    let long = new $util.Long(0, 0, true);
                    object.entityId = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.entityId = options.longs === String ? "0" : 0;
            if (message.entityId != null && message.hasOwnProperty("entityId"))
                if (typeof message.entityId === "number")
                    object.entityId = options.longs === String ? String(message.entityId) : message.entityId;
                else
                    object.entityId = options.longs === String ? $util.Long.prototype.toString.call(message.entityId) : options.longs === Number ? new $util.LongBits(message.entityId.low >>> 0, message.entityId.high >>> 0).toNumber(true) : message.entityId;
            return object;
        };

        /**
         * Converts this S2C_PlayerLeaveWorld to JSON.
         * @function toJSON
         * @memberof proto.S2C_PlayerLeaveWorld
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        S2C_PlayerLeaveWorld.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for S2C_PlayerLeaveWorld
         * @function getTypeUrl
         * @memberof proto.S2C_PlayerLeaveWorld
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        S2C_PlayerLeaveWorld.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.S2C_PlayerLeaveWorld";
        };

        return S2C_PlayerLeaveWorld;
    })();

    proto.S2C_ChunkLoad = (function() {

        /**
         * Properties of a S2C_ChunkLoad.
         * @memberof proto
         * @interface IS2C_ChunkLoad
         * @property {proto.IChunkData|null} [chunk] S2C_ChunkLoad chunk
         */

        /**
         * Constructs a new S2C_ChunkLoad.
         * @memberof proto
         * @classdesc Represents a S2C_ChunkLoad.
         * @implements IS2C_ChunkLoad
         * @constructor
         * @param {proto.IS2C_ChunkLoad=} [properties] Properties to set
         */
        function S2C_ChunkLoad(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * S2C_ChunkLoad chunk.
         * @member {proto.IChunkData|null|undefined} chunk
         * @memberof proto.S2C_ChunkLoad
         * @instance
         */
        S2C_ChunkLoad.prototype.chunk = null;

        /**
         * Creates a new S2C_ChunkLoad instance using the specified properties.
         * @function create
         * @memberof proto.S2C_ChunkLoad
         * @static
         * @param {proto.IS2C_ChunkLoad=} [properties] Properties to set
         * @returns {proto.S2C_ChunkLoad} S2C_ChunkLoad instance
         */
        S2C_ChunkLoad.create = function create(properties) {
            return new S2C_ChunkLoad(properties);
        };

        /**
         * Encodes the specified S2C_ChunkLoad message. Does not implicitly {@link proto.S2C_ChunkLoad.verify|verify} messages.
         * @function encode
         * @memberof proto.S2C_ChunkLoad
         * @static
         * @param {proto.IS2C_ChunkLoad} message S2C_ChunkLoad message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_ChunkLoad.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.chunk != null && Object.hasOwnProperty.call(message, "chunk"))
                $root.proto.ChunkData.encode(message.chunk, writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified S2C_ChunkLoad message, length delimited. Does not implicitly {@link proto.S2C_ChunkLoad.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.S2C_ChunkLoad
         * @static
         * @param {proto.IS2C_ChunkLoad} message S2C_ChunkLoad message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_ChunkLoad.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a S2C_ChunkLoad message from the specified reader or buffer.
         * @function decode
         * @memberof proto.S2C_ChunkLoad
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.S2C_ChunkLoad} S2C_ChunkLoad
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_ChunkLoad.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.S2C_ChunkLoad();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.chunk = $root.proto.ChunkData.decode(reader, reader.uint32());
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a S2C_ChunkLoad message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.S2C_ChunkLoad
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.S2C_ChunkLoad} S2C_ChunkLoad
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_ChunkLoad.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a S2C_ChunkLoad message.
         * @function verify
         * @memberof proto.S2C_ChunkLoad
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        S2C_ChunkLoad.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.chunk != null && message.hasOwnProperty("chunk")) {
                let error = $root.proto.ChunkData.verify(message.chunk);
                if (error)
                    return "chunk." + error;
            }
            return null;
        };

        /**
         * Creates a S2C_ChunkLoad message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.S2C_ChunkLoad
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.S2C_ChunkLoad} S2C_ChunkLoad
         */
        S2C_ChunkLoad.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.S2C_ChunkLoad)
                return object;
            let message = new $root.proto.S2C_ChunkLoad();
            if (object.chunk != null) {
                if (typeof object.chunk !== "object")
                    throw TypeError(".proto.S2C_ChunkLoad.chunk: object expected");
                message.chunk = $root.proto.ChunkData.fromObject(object.chunk);
            }
            return message;
        };

        /**
         * Creates a plain object from a S2C_ChunkLoad message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.S2C_ChunkLoad
         * @static
         * @param {proto.S2C_ChunkLoad} message S2C_ChunkLoad
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        S2C_ChunkLoad.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults)
                object.chunk = null;
            if (message.chunk != null && message.hasOwnProperty("chunk"))
                object.chunk = $root.proto.ChunkData.toObject(message.chunk, options);
            return object;
        };

        /**
         * Converts this S2C_ChunkLoad to JSON.
         * @function toJSON
         * @memberof proto.S2C_ChunkLoad
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        S2C_ChunkLoad.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for S2C_ChunkLoad
         * @function getTypeUrl
         * @memberof proto.S2C_ChunkLoad
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        S2C_ChunkLoad.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.S2C_ChunkLoad";
        };

        return S2C_ChunkLoad;
    })();

    proto.S2C_ChunkUnload = (function() {

        /**
         * Properties of a S2C_ChunkUnload.
         * @memberof proto
         * @interface IS2C_ChunkUnload
         * @property {proto.IChunkCoord|null} [coord] S2C_ChunkUnload coord
         */

        /**
         * Constructs a new S2C_ChunkUnload.
         * @memberof proto
         * @classdesc Represents a S2C_ChunkUnload.
         * @implements IS2C_ChunkUnload
         * @constructor
         * @param {proto.IS2C_ChunkUnload=} [properties] Properties to set
         */
        function S2C_ChunkUnload(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * S2C_ChunkUnload coord.
         * @member {proto.IChunkCoord|null|undefined} coord
         * @memberof proto.S2C_ChunkUnload
         * @instance
         */
        S2C_ChunkUnload.prototype.coord = null;

        /**
         * Creates a new S2C_ChunkUnload instance using the specified properties.
         * @function create
         * @memberof proto.S2C_ChunkUnload
         * @static
         * @param {proto.IS2C_ChunkUnload=} [properties] Properties to set
         * @returns {proto.S2C_ChunkUnload} S2C_ChunkUnload instance
         */
        S2C_ChunkUnload.create = function create(properties) {
            return new S2C_ChunkUnload(properties);
        };

        /**
         * Encodes the specified S2C_ChunkUnload message. Does not implicitly {@link proto.S2C_ChunkUnload.verify|verify} messages.
         * @function encode
         * @memberof proto.S2C_ChunkUnload
         * @static
         * @param {proto.IS2C_ChunkUnload} message S2C_ChunkUnload message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_ChunkUnload.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.coord != null && Object.hasOwnProperty.call(message, "coord"))
                $root.proto.ChunkCoord.encode(message.coord, writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified S2C_ChunkUnload message, length delimited. Does not implicitly {@link proto.S2C_ChunkUnload.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.S2C_ChunkUnload
         * @static
         * @param {proto.IS2C_ChunkUnload} message S2C_ChunkUnload message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_ChunkUnload.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a S2C_ChunkUnload message from the specified reader or buffer.
         * @function decode
         * @memberof proto.S2C_ChunkUnload
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.S2C_ChunkUnload} S2C_ChunkUnload
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_ChunkUnload.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.S2C_ChunkUnload();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.coord = $root.proto.ChunkCoord.decode(reader, reader.uint32());
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a S2C_ChunkUnload message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.S2C_ChunkUnload
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.S2C_ChunkUnload} S2C_ChunkUnload
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_ChunkUnload.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a S2C_ChunkUnload message.
         * @function verify
         * @memberof proto.S2C_ChunkUnload
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        S2C_ChunkUnload.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.coord != null && message.hasOwnProperty("coord")) {
                let error = $root.proto.ChunkCoord.verify(message.coord);
                if (error)
                    return "coord." + error;
            }
            return null;
        };

        /**
         * Creates a S2C_ChunkUnload message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.S2C_ChunkUnload
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.S2C_ChunkUnload} S2C_ChunkUnload
         */
        S2C_ChunkUnload.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.S2C_ChunkUnload)
                return object;
            let message = new $root.proto.S2C_ChunkUnload();
            if (object.coord != null) {
                if (typeof object.coord !== "object")
                    throw TypeError(".proto.S2C_ChunkUnload.coord: object expected");
                message.coord = $root.proto.ChunkCoord.fromObject(object.coord);
            }
            return message;
        };

        /**
         * Creates a plain object from a S2C_ChunkUnload message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.S2C_ChunkUnload
         * @static
         * @param {proto.S2C_ChunkUnload} message S2C_ChunkUnload
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        S2C_ChunkUnload.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults)
                object.coord = null;
            if (message.coord != null && message.hasOwnProperty("coord"))
                object.coord = $root.proto.ChunkCoord.toObject(message.coord, options);
            return object;
        };

        /**
         * Converts this S2C_ChunkUnload to JSON.
         * @function toJSON
         * @memberof proto.S2C_ChunkUnload
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        S2C_ChunkUnload.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for S2C_ChunkUnload
         * @function getTypeUrl
         * @memberof proto.S2C_ChunkUnload
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        S2C_ChunkUnload.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.S2C_ChunkUnload";
        };

        return S2C_ChunkUnload;
    })();

    proto.S2C_ObjectSpawn = (function() {

        /**
         * Properties of a S2C_ObjectSpawn.
         * @memberof proto
         * @interface IS2C_ObjectSpawn
         * @property {number|Long|null} [entityId] S2C_ObjectSpawn entityId
         * @property {number|null} [objectType] S2C_ObjectSpawn objectType
         * @property {string|null} [resourcePath] S2C_ObjectSpawn resourcePath
         * @property {proto.IEntityPosition|null} [position] S2C_ObjectSpawn position
         */

        /**
         * Constructs a new S2C_ObjectSpawn.
         * @memberof proto
         * @classdesc Represents a S2C_ObjectSpawn.
         * @implements IS2C_ObjectSpawn
         * @constructor
         * @param {proto.IS2C_ObjectSpawn=} [properties] Properties to set
         */
        function S2C_ObjectSpawn(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * S2C_ObjectSpawn entityId.
         * @member {number|Long} entityId
         * @memberof proto.S2C_ObjectSpawn
         * @instance
         */
        S2C_ObjectSpawn.prototype.entityId = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * S2C_ObjectSpawn objectType.
         * @member {number} objectType
         * @memberof proto.S2C_ObjectSpawn
         * @instance
         */
        S2C_ObjectSpawn.prototype.objectType = 0;

        /**
         * S2C_ObjectSpawn resourcePath.
         * @member {string} resourcePath
         * @memberof proto.S2C_ObjectSpawn
         * @instance
         */
        S2C_ObjectSpawn.prototype.resourcePath = "";

        /**
         * S2C_ObjectSpawn position.
         * @member {proto.IEntityPosition|null|undefined} position
         * @memberof proto.S2C_ObjectSpawn
         * @instance
         */
        S2C_ObjectSpawn.prototype.position = null;

        /**
         * Creates a new S2C_ObjectSpawn instance using the specified properties.
         * @function create
         * @memberof proto.S2C_ObjectSpawn
         * @static
         * @param {proto.IS2C_ObjectSpawn=} [properties] Properties to set
         * @returns {proto.S2C_ObjectSpawn} S2C_ObjectSpawn instance
         */
        S2C_ObjectSpawn.create = function create(properties) {
            return new S2C_ObjectSpawn(properties);
        };

        /**
         * Encodes the specified S2C_ObjectSpawn message. Does not implicitly {@link proto.S2C_ObjectSpawn.verify|verify} messages.
         * @function encode
         * @memberof proto.S2C_ObjectSpawn
         * @static
         * @param {proto.IS2C_ObjectSpawn} message S2C_ObjectSpawn message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_ObjectSpawn.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.entityId != null && Object.hasOwnProperty.call(message, "entityId"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint64(message.entityId);
            if (message.objectType != null && Object.hasOwnProperty.call(message, "objectType"))
                writer.uint32(/* id 2, wireType 0 =*/16).int32(message.objectType);
            if (message.resourcePath != null && Object.hasOwnProperty.call(message, "resourcePath"))
                writer.uint32(/* id 3, wireType 2 =*/26).string(message.resourcePath);
            if (message.position != null && Object.hasOwnProperty.call(message, "position"))
                $root.proto.EntityPosition.encode(message.position, writer.uint32(/* id 4, wireType 2 =*/34).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified S2C_ObjectSpawn message, length delimited. Does not implicitly {@link proto.S2C_ObjectSpawn.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.S2C_ObjectSpawn
         * @static
         * @param {proto.IS2C_ObjectSpawn} message S2C_ObjectSpawn message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_ObjectSpawn.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a S2C_ObjectSpawn message from the specified reader or buffer.
         * @function decode
         * @memberof proto.S2C_ObjectSpawn
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.S2C_ObjectSpawn} S2C_ObjectSpawn
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_ObjectSpawn.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.S2C_ObjectSpawn();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.entityId = reader.uint64();
                        break;
                    }
                case 2: {
                        message.objectType = reader.int32();
                        break;
                    }
                case 3: {
                        message.resourcePath = reader.string();
                        break;
                    }
                case 4: {
                        message.position = $root.proto.EntityPosition.decode(reader, reader.uint32());
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a S2C_ObjectSpawn message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.S2C_ObjectSpawn
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.S2C_ObjectSpawn} S2C_ObjectSpawn
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_ObjectSpawn.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a S2C_ObjectSpawn message.
         * @function verify
         * @memberof proto.S2C_ObjectSpawn
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        S2C_ObjectSpawn.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.entityId != null && message.hasOwnProperty("entityId"))
                if (!$util.isInteger(message.entityId) && !(message.entityId && $util.isInteger(message.entityId.low) && $util.isInteger(message.entityId.high)))
                    return "entityId: integer|Long expected";
            if (message.objectType != null && message.hasOwnProperty("objectType"))
                if (!$util.isInteger(message.objectType))
                    return "objectType: integer expected";
            if (message.resourcePath != null && message.hasOwnProperty("resourcePath"))
                if (!$util.isString(message.resourcePath))
                    return "resourcePath: string expected";
            if (message.position != null && message.hasOwnProperty("position")) {
                let error = $root.proto.EntityPosition.verify(message.position);
                if (error)
                    return "position." + error;
            }
            return null;
        };

        /**
         * Creates a S2C_ObjectSpawn message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.S2C_ObjectSpawn
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.S2C_ObjectSpawn} S2C_ObjectSpawn
         */
        S2C_ObjectSpawn.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.S2C_ObjectSpawn)
                return object;
            let message = new $root.proto.S2C_ObjectSpawn();
            if (object.entityId != null)
                if ($util.Long)
                    (message.entityId = $util.Long.fromValue(object.entityId)).unsigned = true;
                else if (typeof object.entityId === "string")
                    message.entityId = parseInt(object.entityId, 10);
                else if (typeof object.entityId === "number")
                    message.entityId = object.entityId;
                else if (typeof object.entityId === "object")
                    message.entityId = new $util.LongBits(object.entityId.low >>> 0, object.entityId.high >>> 0).toNumber(true);
            if (object.objectType != null)
                message.objectType = object.objectType | 0;
            if (object.resourcePath != null)
                message.resourcePath = String(object.resourcePath);
            if (object.position != null) {
                if (typeof object.position !== "object")
                    throw TypeError(".proto.S2C_ObjectSpawn.position: object expected");
                message.position = $root.proto.EntityPosition.fromObject(object.position);
            }
            return message;
        };

        /**
         * Creates a plain object from a S2C_ObjectSpawn message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.S2C_ObjectSpawn
         * @static
         * @param {proto.S2C_ObjectSpawn} message S2C_ObjectSpawn
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        S2C_ObjectSpawn.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                if ($util.Long) {
                    let long = new $util.Long(0, 0, true);
                    object.entityId = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.entityId = options.longs === String ? "0" : 0;
                object.objectType = 0;
                object.resourcePath = "";
                object.position = null;
            }
            if (message.entityId != null && message.hasOwnProperty("entityId"))
                if (typeof message.entityId === "number")
                    object.entityId = options.longs === String ? String(message.entityId) : message.entityId;
                else
                    object.entityId = options.longs === String ? $util.Long.prototype.toString.call(message.entityId) : options.longs === Number ? new $util.LongBits(message.entityId.low >>> 0, message.entityId.high >>> 0).toNumber(true) : message.entityId;
            if (message.objectType != null && message.hasOwnProperty("objectType"))
                object.objectType = message.objectType;
            if (message.resourcePath != null && message.hasOwnProperty("resourcePath"))
                object.resourcePath = message.resourcePath;
            if (message.position != null && message.hasOwnProperty("position"))
                object.position = $root.proto.EntityPosition.toObject(message.position, options);
            return object;
        };

        /**
         * Converts this S2C_ObjectSpawn to JSON.
         * @function toJSON
         * @memberof proto.S2C_ObjectSpawn
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        S2C_ObjectSpawn.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for S2C_ObjectSpawn
         * @function getTypeUrl
         * @memberof proto.S2C_ObjectSpawn
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        S2C_ObjectSpawn.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.S2C_ObjectSpawn";
        };

        return S2C_ObjectSpawn;
    })();

    proto.S2C_ObjectDespawn = (function() {

        /**
         * Properties of a S2C_ObjectDespawn.
         * @memberof proto
         * @interface IS2C_ObjectDespawn
         * @property {number|Long|null} [entityId] S2C_ObjectDespawn entityId
         */

        /**
         * Constructs a new S2C_ObjectDespawn.
         * @memberof proto
         * @classdesc Represents a S2C_ObjectDespawn.
         * @implements IS2C_ObjectDespawn
         * @constructor
         * @param {proto.IS2C_ObjectDespawn=} [properties] Properties to set
         */
        function S2C_ObjectDespawn(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * S2C_ObjectDespawn entityId.
         * @member {number|Long} entityId
         * @memberof proto.S2C_ObjectDespawn
         * @instance
         */
        S2C_ObjectDespawn.prototype.entityId = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * Creates a new S2C_ObjectDespawn instance using the specified properties.
         * @function create
         * @memberof proto.S2C_ObjectDespawn
         * @static
         * @param {proto.IS2C_ObjectDespawn=} [properties] Properties to set
         * @returns {proto.S2C_ObjectDespawn} S2C_ObjectDespawn instance
         */
        S2C_ObjectDespawn.create = function create(properties) {
            return new S2C_ObjectDespawn(properties);
        };

        /**
         * Encodes the specified S2C_ObjectDespawn message. Does not implicitly {@link proto.S2C_ObjectDespawn.verify|verify} messages.
         * @function encode
         * @memberof proto.S2C_ObjectDespawn
         * @static
         * @param {proto.IS2C_ObjectDespawn} message S2C_ObjectDespawn message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_ObjectDespawn.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.entityId != null && Object.hasOwnProperty.call(message, "entityId"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint64(message.entityId);
            return writer;
        };

        /**
         * Encodes the specified S2C_ObjectDespawn message, length delimited. Does not implicitly {@link proto.S2C_ObjectDespawn.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.S2C_ObjectDespawn
         * @static
         * @param {proto.IS2C_ObjectDespawn} message S2C_ObjectDespawn message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_ObjectDespawn.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a S2C_ObjectDespawn message from the specified reader or buffer.
         * @function decode
         * @memberof proto.S2C_ObjectDespawn
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.S2C_ObjectDespawn} S2C_ObjectDespawn
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_ObjectDespawn.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.S2C_ObjectDespawn();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.entityId = reader.uint64();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a S2C_ObjectDespawn message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.S2C_ObjectDespawn
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.S2C_ObjectDespawn} S2C_ObjectDespawn
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_ObjectDespawn.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a S2C_ObjectDespawn message.
         * @function verify
         * @memberof proto.S2C_ObjectDespawn
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        S2C_ObjectDespawn.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.entityId != null && message.hasOwnProperty("entityId"))
                if (!$util.isInteger(message.entityId) && !(message.entityId && $util.isInteger(message.entityId.low) && $util.isInteger(message.entityId.high)))
                    return "entityId: integer|Long expected";
            return null;
        };

        /**
         * Creates a S2C_ObjectDespawn message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.S2C_ObjectDespawn
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.S2C_ObjectDespawn} S2C_ObjectDespawn
         */
        S2C_ObjectDespawn.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.S2C_ObjectDespawn)
                return object;
            let message = new $root.proto.S2C_ObjectDespawn();
            if (object.entityId != null)
                if ($util.Long)
                    (message.entityId = $util.Long.fromValue(object.entityId)).unsigned = true;
                else if (typeof object.entityId === "string")
                    message.entityId = parseInt(object.entityId, 10);
                else if (typeof object.entityId === "number")
                    message.entityId = object.entityId;
                else if (typeof object.entityId === "object")
                    message.entityId = new $util.LongBits(object.entityId.low >>> 0, object.entityId.high >>> 0).toNumber(true);
            return message;
        };

        /**
         * Creates a plain object from a S2C_ObjectDespawn message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.S2C_ObjectDespawn
         * @static
         * @param {proto.S2C_ObjectDespawn} message S2C_ObjectDespawn
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        S2C_ObjectDespawn.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults)
                if ($util.Long) {
                    let long = new $util.Long(0, 0, true);
                    object.entityId = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.entityId = options.longs === String ? "0" : 0;
            if (message.entityId != null && message.hasOwnProperty("entityId"))
                if (typeof message.entityId === "number")
                    object.entityId = options.longs === String ? String(message.entityId) : message.entityId;
                else
                    object.entityId = options.longs === String ? $util.Long.prototype.toString.call(message.entityId) : options.longs === Number ? new $util.LongBits(message.entityId.low >>> 0, message.entityId.high >>> 0).toNumber(true) : message.entityId;
            return object;
        };

        /**
         * Converts this S2C_ObjectDespawn to JSON.
         * @function toJSON
         * @memberof proto.S2C_ObjectDespawn
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        S2C_ObjectDespawn.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for S2C_ObjectDespawn
         * @function getTypeUrl
         * @memberof proto.S2C_ObjectDespawn
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        S2C_ObjectDespawn.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.S2C_ObjectDespawn";
        };

        return S2C_ObjectDespawn;
    })();

    proto.S2C_ObjectMove = (function() {

        /**
         * Properties of a S2C_ObjectMove.
         * @memberof proto
         * @interface IS2C_ObjectMove
         * @property {number|Long|null} [entityId] S2C_ObjectMove entityId
         * @property {proto.IEntityMovement|null} [movement] S2C_ObjectMove movement
         */

        /**
         * Constructs a new S2C_ObjectMove.
         * @memberof proto
         * @classdesc Represents a S2C_ObjectMove.
         * @implements IS2C_ObjectMove
         * @constructor
         * @param {proto.IS2C_ObjectMove=} [properties] Properties to set
         */
        function S2C_ObjectMove(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * S2C_ObjectMove entityId.
         * @member {number|Long} entityId
         * @memberof proto.S2C_ObjectMove
         * @instance
         */
        S2C_ObjectMove.prototype.entityId = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * S2C_ObjectMove movement.
         * @member {proto.IEntityMovement|null|undefined} movement
         * @memberof proto.S2C_ObjectMove
         * @instance
         */
        S2C_ObjectMove.prototype.movement = null;

        /**
         * Creates a new S2C_ObjectMove instance using the specified properties.
         * @function create
         * @memberof proto.S2C_ObjectMove
         * @static
         * @param {proto.IS2C_ObjectMove=} [properties] Properties to set
         * @returns {proto.S2C_ObjectMove} S2C_ObjectMove instance
         */
        S2C_ObjectMove.create = function create(properties) {
            return new S2C_ObjectMove(properties);
        };

        /**
         * Encodes the specified S2C_ObjectMove message. Does not implicitly {@link proto.S2C_ObjectMove.verify|verify} messages.
         * @function encode
         * @memberof proto.S2C_ObjectMove
         * @static
         * @param {proto.IS2C_ObjectMove} message S2C_ObjectMove message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_ObjectMove.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.entityId != null && Object.hasOwnProperty.call(message, "entityId"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint64(message.entityId);
            if (message.movement != null && Object.hasOwnProperty.call(message, "movement"))
                $root.proto.EntityMovement.encode(message.movement, writer.uint32(/* id 2, wireType 2 =*/18).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified S2C_ObjectMove message, length delimited. Does not implicitly {@link proto.S2C_ObjectMove.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.S2C_ObjectMove
         * @static
         * @param {proto.IS2C_ObjectMove} message S2C_ObjectMove message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_ObjectMove.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a S2C_ObjectMove message from the specified reader or buffer.
         * @function decode
         * @memberof proto.S2C_ObjectMove
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.S2C_ObjectMove} S2C_ObjectMove
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_ObjectMove.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.S2C_ObjectMove();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.entityId = reader.uint64();
                        break;
                    }
                case 2: {
                        message.movement = $root.proto.EntityMovement.decode(reader, reader.uint32());
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a S2C_ObjectMove message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.S2C_ObjectMove
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.S2C_ObjectMove} S2C_ObjectMove
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_ObjectMove.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a S2C_ObjectMove message.
         * @function verify
         * @memberof proto.S2C_ObjectMove
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        S2C_ObjectMove.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.entityId != null && message.hasOwnProperty("entityId"))
                if (!$util.isInteger(message.entityId) && !(message.entityId && $util.isInteger(message.entityId.low) && $util.isInteger(message.entityId.high)))
                    return "entityId: integer|Long expected";
            if (message.movement != null && message.hasOwnProperty("movement")) {
                let error = $root.proto.EntityMovement.verify(message.movement);
                if (error)
                    return "movement." + error;
            }
            return null;
        };

        /**
         * Creates a S2C_ObjectMove message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.S2C_ObjectMove
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.S2C_ObjectMove} S2C_ObjectMove
         */
        S2C_ObjectMove.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.S2C_ObjectMove)
                return object;
            let message = new $root.proto.S2C_ObjectMove();
            if (object.entityId != null)
                if ($util.Long)
                    (message.entityId = $util.Long.fromValue(object.entityId)).unsigned = true;
                else if (typeof object.entityId === "string")
                    message.entityId = parseInt(object.entityId, 10);
                else if (typeof object.entityId === "number")
                    message.entityId = object.entityId;
                else if (typeof object.entityId === "object")
                    message.entityId = new $util.LongBits(object.entityId.low >>> 0, object.entityId.high >>> 0).toNumber(true);
            if (object.movement != null) {
                if (typeof object.movement !== "object")
                    throw TypeError(".proto.S2C_ObjectMove.movement: object expected");
                message.movement = $root.proto.EntityMovement.fromObject(object.movement);
            }
            return message;
        };

        /**
         * Creates a plain object from a S2C_ObjectMove message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.S2C_ObjectMove
         * @static
         * @param {proto.S2C_ObjectMove} message S2C_ObjectMove
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        S2C_ObjectMove.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                if ($util.Long) {
                    let long = new $util.Long(0, 0, true);
                    object.entityId = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.entityId = options.longs === String ? "0" : 0;
                object.movement = null;
            }
            if (message.entityId != null && message.hasOwnProperty("entityId"))
                if (typeof message.entityId === "number")
                    object.entityId = options.longs === String ? String(message.entityId) : message.entityId;
                else
                    object.entityId = options.longs === String ? $util.Long.prototype.toString.call(message.entityId) : options.longs === Number ? new $util.LongBits(message.entityId.low >>> 0, message.entityId.high >>> 0).toNumber(true) : message.entityId;
            if (message.movement != null && message.hasOwnProperty("movement"))
                object.movement = $root.proto.EntityMovement.toObject(message.movement, options);
            return object;
        };

        /**
         * Converts this S2C_ObjectMove to JSON.
         * @function toJSON
         * @memberof proto.S2C_ObjectMove
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        S2C_ObjectMove.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for S2C_ObjectMove
         * @function getTypeUrl
         * @memberof proto.S2C_ObjectMove
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        S2C_ObjectMove.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.S2C_ObjectMove";
        };

        return S2C_ObjectMove;
    })();

    proto.S2C_InventoryOpResult = (function() {

        /**
         * Properties of a S2C_InventoryOpResult.
         * @memberof proto
         * @interface IS2C_InventoryOpResult
         * @property {number|Long|null} [opId] S2C_InventoryOpResult opId
         * @property {boolean|null} [success] S2C_InventoryOpResult success
         * @property {proto.ErrorCode|null} [error] S2C_InventoryOpResult error
         * @property {string|null} [message] S2C_InventoryOpResult message
         * @property {Array.<proto.IInventoryState>|null} [updated] S2C_InventoryOpResult updated
         * @property {number|Long|null} [spawnedDroppedEntityId] S2C_InventoryOpResult spawnedDroppedEntityId
         * @property {number|Long|null} [despawnedDroppedEntityId] S2C_InventoryOpResult despawnedDroppedEntityId
         */

        /**
         * Constructs a new S2C_InventoryOpResult.
         * @memberof proto
         * @classdesc Represents a S2C_InventoryOpResult.
         * @implements IS2C_InventoryOpResult
         * @constructor
         * @param {proto.IS2C_InventoryOpResult=} [properties] Properties to set
         */
        function S2C_InventoryOpResult(properties) {
            this.updated = [];
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * S2C_InventoryOpResult opId.
         * @member {number|Long} opId
         * @memberof proto.S2C_InventoryOpResult
         * @instance
         */
        S2C_InventoryOpResult.prototype.opId = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * S2C_InventoryOpResult success.
         * @member {boolean} success
         * @memberof proto.S2C_InventoryOpResult
         * @instance
         */
        S2C_InventoryOpResult.prototype.success = false;

        /**
         * S2C_InventoryOpResult error.
         * @member {proto.ErrorCode} error
         * @memberof proto.S2C_InventoryOpResult
         * @instance
         */
        S2C_InventoryOpResult.prototype.error = 0;

        /**
         * S2C_InventoryOpResult message.
         * @member {string} message
         * @memberof proto.S2C_InventoryOpResult
         * @instance
         */
        S2C_InventoryOpResult.prototype.message = "";

        /**
         * S2C_InventoryOpResult updated.
         * @member {Array.<proto.IInventoryState>} updated
         * @memberof proto.S2C_InventoryOpResult
         * @instance
         */
        S2C_InventoryOpResult.prototype.updated = $util.emptyArray;

        /**
         * S2C_InventoryOpResult spawnedDroppedEntityId.
         * @member {number|Long|null|undefined} spawnedDroppedEntityId
         * @memberof proto.S2C_InventoryOpResult
         * @instance
         */
        S2C_InventoryOpResult.prototype.spawnedDroppedEntityId = null;

        /**
         * S2C_InventoryOpResult despawnedDroppedEntityId.
         * @member {number|Long|null|undefined} despawnedDroppedEntityId
         * @memberof proto.S2C_InventoryOpResult
         * @instance
         */
        S2C_InventoryOpResult.prototype.despawnedDroppedEntityId = null;

        // OneOf field names bound to virtual getters and setters
        let $oneOfFields;

        // Virtual OneOf for proto3 optional field
        Object.defineProperty(S2C_InventoryOpResult.prototype, "_spawnedDroppedEntityId", {
            get: $util.oneOfGetter($oneOfFields = ["spawnedDroppedEntityId"]),
            set: $util.oneOfSetter($oneOfFields)
        });

        // Virtual OneOf for proto3 optional field
        Object.defineProperty(S2C_InventoryOpResult.prototype, "_despawnedDroppedEntityId", {
            get: $util.oneOfGetter($oneOfFields = ["despawnedDroppedEntityId"]),
            set: $util.oneOfSetter($oneOfFields)
        });

        /**
         * Creates a new S2C_InventoryOpResult instance using the specified properties.
         * @function create
         * @memberof proto.S2C_InventoryOpResult
         * @static
         * @param {proto.IS2C_InventoryOpResult=} [properties] Properties to set
         * @returns {proto.S2C_InventoryOpResult} S2C_InventoryOpResult instance
         */
        S2C_InventoryOpResult.create = function create(properties) {
            return new S2C_InventoryOpResult(properties);
        };

        /**
         * Encodes the specified S2C_InventoryOpResult message. Does not implicitly {@link proto.S2C_InventoryOpResult.verify|verify} messages.
         * @function encode
         * @memberof proto.S2C_InventoryOpResult
         * @static
         * @param {proto.IS2C_InventoryOpResult} message S2C_InventoryOpResult message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_InventoryOpResult.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.opId != null && Object.hasOwnProperty.call(message, "opId"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint64(message.opId);
            if (message.success != null && Object.hasOwnProperty.call(message, "success"))
                writer.uint32(/* id 2, wireType 0 =*/16).bool(message.success);
            if (message.error != null && Object.hasOwnProperty.call(message, "error"))
                writer.uint32(/* id 3, wireType 0 =*/24).int32(message.error);
            if (message.message != null && Object.hasOwnProperty.call(message, "message"))
                writer.uint32(/* id 4, wireType 2 =*/34).string(message.message);
            if (message.updated != null && message.updated.length)
                for (let i = 0; i < message.updated.length; ++i)
                    $root.proto.InventoryState.encode(message.updated[i], writer.uint32(/* id 10, wireType 2 =*/82).fork()).ldelim();
            if (message.spawnedDroppedEntityId != null && Object.hasOwnProperty.call(message, "spawnedDroppedEntityId"))
                writer.uint32(/* id 20, wireType 0 =*/160).uint64(message.spawnedDroppedEntityId);
            if (message.despawnedDroppedEntityId != null && Object.hasOwnProperty.call(message, "despawnedDroppedEntityId"))
                writer.uint32(/* id 21, wireType 0 =*/168).uint64(message.despawnedDroppedEntityId);
            return writer;
        };

        /**
         * Encodes the specified S2C_InventoryOpResult message, length delimited. Does not implicitly {@link proto.S2C_InventoryOpResult.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.S2C_InventoryOpResult
         * @static
         * @param {proto.IS2C_InventoryOpResult} message S2C_InventoryOpResult message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_InventoryOpResult.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a S2C_InventoryOpResult message from the specified reader or buffer.
         * @function decode
         * @memberof proto.S2C_InventoryOpResult
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.S2C_InventoryOpResult} S2C_InventoryOpResult
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_InventoryOpResult.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.S2C_InventoryOpResult();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.opId = reader.uint64();
                        break;
                    }
                case 2: {
                        message.success = reader.bool();
                        break;
                    }
                case 3: {
                        message.error = reader.int32();
                        break;
                    }
                case 4: {
                        message.message = reader.string();
                        break;
                    }
                case 10: {
                        if (!(message.updated && message.updated.length))
                            message.updated = [];
                        message.updated.push($root.proto.InventoryState.decode(reader, reader.uint32()));
                        break;
                    }
                case 20: {
                        message.spawnedDroppedEntityId = reader.uint64();
                        break;
                    }
                case 21: {
                        message.despawnedDroppedEntityId = reader.uint64();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a S2C_InventoryOpResult message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.S2C_InventoryOpResult
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.S2C_InventoryOpResult} S2C_InventoryOpResult
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_InventoryOpResult.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a S2C_InventoryOpResult message.
         * @function verify
         * @memberof proto.S2C_InventoryOpResult
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        S2C_InventoryOpResult.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            let properties = {};
            if (message.opId != null && message.hasOwnProperty("opId"))
                if (!$util.isInteger(message.opId) && !(message.opId && $util.isInteger(message.opId.low) && $util.isInteger(message.opId.high)))
                    return "opId: integer|Long expected";
            if (message.success != null && message.hasOwnProperty("success"))
                if (typeof message.success !== "boolean")
                    return "success: boolean expected";
            if (message.error != null && message.hasOwnProperty("error"))
                switch (message.error) {
                default:
                    return "error: enum value expected";
                case 0:
                case 1:
                case 2:
                case 3:
                case 4:
                case 5:
                case 6:
                case 7:
                case 8:
                case 9:
                case 10:
                case 11:
                case 12:
                case 13:
                case 14:
                case 15:
                case 16:
                    break;
                }
            if (message.message != null && message.hasOwnProperty("message"))
                if (!$util.isString(message.message))
                    return "message: string expected";
            if (message.updated != null && message.hasOwnProperty("updated")) {
                if (!Array.isArray(message.updated))
                    return "updated: array expected";
                for (let i = 0; i < message.updated.length; ++i) {
                    let error = $root.proto.InventoryState.verify(message.updated[i]);
                    if (error)
                        return "updated." + error;
                }
            }
            if (message.spawnedDroppedEntityId != null && message.hasOwnProperty("spawnedDroppedEntityId")) {
                properties._spawnedDroppedEntityId = 1;
                if (!$util.isInteger(message.spawnedDroppedEntityId) && !(message.spawnedDroppedEntityId && $util.isInteger(message.spawnedDroppedEntityId.low) && $util.isInteger(message.spawnedDroppedEntityId.high)))
                    return "spawnedDroppedEntityId: integer|Long expected";
            }
            if (message.despawnedDroppedEntityId != null && message.hasOwnProperty("despawnedDroppedEntityId")) {
                properties._despawnedDroppedEntityId = 1;
                if (!$util.isInteger(message.despawnedDroppedEntityId) && !(message.despawnedDroppedEntityId && $util.isInteger(message.despawnedDroppedEntityId.low) && $util.isInteger(message.despawnedDroppedEntityId.high)))
                    return "despawnedDroppedEntityId: integer|Long expected";
            }
            return null;
        };

        /**
         * Creates a S2C_InventoryOpResult message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.S2C_InventoryOpResult
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.S2C_InventoryOpResult} S2C_InventoryOpResult
         */
        S2C_InventoryOpResult.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.S2C_InventoryOpResult)
                return object;
            let message = new $root.proto.S2C_InventoryOpResult();
            if (object.opId != null)
                if ($util.Long)
                    (message.opId = $util.Long.fromValue(object.opId)).unsigned = true;
                else if (typeof object.opId === "string")
                    message.opId = parseInt(object.opId, 10);
                else if (typeof object.opId === "number")
                    message.opId = object.opId;
                else if (typeof object.opId === "object")
                    message.opId = new $util.LongBits(object.opId.low >>> 0, object.opId.high >>> 0).toNumber(true);
            if (object.success != null)
                message.success = Boolean(object.success);
            switch (object.error) {
            default:
                if (typeof object.error === "number") {
                    message.error = object.error;
                    break;
                }
                break;
            case "ERROR_CODE_NONE":
            case 0:
                message.error = 0;
                break;
            case "ERROR_CODE_INVALID_REQUEST":
            case 1:
                message.error = 1;
                break;
            case "ERROR_CODE_NOT_AUTHENTICATED":
            case 2:
                message.error = 2;
                break;
            case "ERROR_CODE_ENTITY_NOT_FOUND":
            case 3:
                message.error = 3;
                break;
            case "ERROR_CODE_OUT_OF_RANGE":
            case 4:
                message.error = 4;
                break;
            case "ERROR_CODE_INSUFFICIENT_RESOURCES":
            case 5:
                message.error = 5;
                break;
            case "ERROR_CODE_INVENTORY_FULL":
            case 6:
                message.error = 6;
                break;
            case "ERROR_CODE_CANNOT_INTERACT":
            case 7:
                message.error = 7;
                break;
            case "ERROR_CODE_COOLDOWN_ACTIVE":
            case 8:
                message.error = 8;
                break;
            case "ERROR_CODE_INSUFFICIENT_STAMINA":
            case 9:
                message.error = 9;
                break;
            case "ERROR_CODE_TARGET_INVALID":
            case 10:
                message.error = 10;
                break;
            case "ERROR_CODE_PATH_BLOCKED":
            case 11:
                message.error = 11;
                break;
            case "ERROR_CODE_TIMEOUT_EXCEEDED":
            case 12:
                message.error = 12;
                break;
            case "ERROR_CODE_BUILDING_INCOMPLETE":
            case 13:
                message.error = 13;
                break;
            case "ERROR_CODE_RECIPE_UNKNOWN":
            case 14:
                message.error = 14;
                break;
            case "ERROR_PACKET_PER_SECOND_LIMIT_THRESHOLDED":
            case 15:
                message.error = 15;
                break;
            case "ERROR_CODE_INTERNAL_ERROR":
            case 16:
                message.error = 16;
                break;
            }
            if (object.message != null)
                message.message = String(object.message);
            if (object.updated) {
                if (!Array.isArray(object.updated))
                    throw TypeError(".proto.S2C_InventoryOpResult.updated: array expected");
                message.updated = [];
                for (let i = 0; i < object.updated.length; ++i) {
                    if (typeof object.updated[i] !== "object")
                        throw TypeError(".proto.S2C_InventoryOpResult.updated: object expected");
                    message.updated[i] = $root.proto.InventoryState.fromObject(object.updated[i]);
                }
            }
            if (object.spawnedDroppedEntityId != null)
                if ($util.Long)
                    (message.spawnedDroppedEntityId = $util.Long.fromValue(object.spawnedDroppedEntityId)).unsigned = true;
                else if (typeof object.spawnedDroppedEntityId === "string")
                    message.spawnedDroppedEntityId = parseInt(object.spawnedDroppedEntityId, 10);
                else if (typeof object.spawnedDroppedEntityId === "number")
                    message.spawnedDroppedEntityId = object.spawnedDroppedEntityId;
                else if (typeof object.spawnedDroppedEntityId === "object")
                    message.spawnedDroppedEntityId = new $util.LongBits(object.spawnedDroppedEntityId.low >>> 0, object.spawnedDroppedEntityId.high >>> 0).toNumber(true);
            if (object.despawnedDroppedEntityId != null)
                if ($util.Long)
                    (message.despawnedDroppedEntityId = $util.Long.fromValue(object.despawnedDroppedEntityId)).unsigned = true;
                else if (typeof object.despawnedDroppedEntityId === "string")
                    message.despawnedDroppedEntityId = parseInt(object.despawnedDroppedEntityId, 10);
                else if (typeof object.despawnedDroppedEntityId === "number")
                    message.despawnedDroppedEntityId = object.despawnedDroppedEntityId;
                else if (typeof object.despawnedDroppedEntityId === "object")
                    message.despawnedDroppedEntityId = new $util.LongBits(object.despawnedDroppedEntityId.low >>> 0, object.despawnedDroppedEntityId.high >>> 0).toNumber(true);
            return message;
        };

        /**
         * Creates a plain object from a S2C_InventoryOpResult message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.S2C_InventoryOpResult
         * @static
         * @param {proto.S2C_InventoryOpResult} message S2C_InventoryOpResult
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        S2C_InventoryOpResult.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.arrays || options.defaults)
                object.updated = [];
            if (options.defaults) {
                if ($util.Long) {
                    let long = new $util.Long(0, 0, true);
                    object.opId = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.opId = options.longs === String ? "0" : 0;
                object.success = false;
                object.error = options.enums === String ? "ERROR_CODE_NONE" : 0;
                object.message = "";
            }
            if (message.opId != null && message.hasOwnProperty("opId"))
                if (typeof message.opId === "number")
                    object.opId = options.longs === String ? String(message.opId) : message.opId;
                else
                    object.opId = options.longs === String ? $util.Long.prototype.toString.call(message.opId) : options.longs === Number ? new $util.LongBits(message.opId.low >>> 0, message.opId.high >>> 0).toNumber(true) : message.opId;
            if (message.success != null && message.hasOwnProperty("success"))
                object.success = message.success;
            if (message.error != null && message.hasOwnProperty("error"))
                object.error = options.enums === String ? $root.proto.ErrorCode[message.error] === undefined ? message.error : $root.proto.ErrorCode[message.error] : message.error;
            if (message.message != null && message.hasOwnProperty("message"))
                object.message = message.message;
            if (message.updated && message.updated.length) {
                object.updated = [];
                for (let j = 0; j < message.updated.length; ++j)
                    object.updated[j] = $root.proto.InventoryState.toObject(message.updated[j], options);
            }
            if (message.spawnedDroppedEntityId != null && message.hasOwnProperty("spawnedDroppedEntityId")) {
                if (typeof message.spawnedDroppedEntityId === "number")
                    object.spawnedDroppedEntityId = options.longs === String ? String(message.spawnedDroppedEntityId) : message.spawnedDroppedEntityId;
                else
                    object.spawnedDroppedEntityId = options.longs === String ? $util.Long.prototype.toString.call(message.spawnedDroppedEntityId) : options.longs === Number ? new $util.LongBits(message.spawnedDroppedEntityId.low >>> 0, message.spawnedDroppedEntityId.high >>> 0).toNumber(true) : message.spawnedDroppedEntityId;
                if (options.oneofs)
                    object._spawnedDroppedEntityId = "spawnedDroppedEntityId";
            }
            if (message.despawnedDroppedEntityId != null && message.hasOwnProperty("despawnedDroppedEntityId")) {
                if (typeof message.despawnedDroppedEntityId === "number")
                    object.despawnedDroppedEntityId = options.longs === String ? String(message.despawnedDroppedEntityId) : message.despawnedDroppedEntityId;
                else
                    object.despawnedDroppedEntityId = options.longs === String ? $util.Long.prototype.toString.call(message.despawnedDroppedEntityId) : options.longs === Number ? new $util.LongBits(message.despawnedDroppedEntityId.low >>> 0, message.despawnedDroppedEntityId.high >>> 0).toNumber(true) : message.despawnedDroppedEntityId;
                if (options.oneofs)
                    object._despawnedDroppedEntityId = "despawnedDroppedEntityId";
            }
            return object;
        };

        /**
         * Converts this S2C_InventoryOpResult to JSON.
         * @function toJSON
         * @memberof proto.S2C_InventoryOpResult
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        S2C_InventoryOpResult.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for S2C_InventoryOpResult
         * @function getTypeUrl
         * @memberof proto.S2C_InventoryOpResult
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        S2C_InventoryOpResult.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.S2C_InventoryOpResult";
        };

        return S2C_InventoryOpResult;
    })();

    proto.S2C_InventoryUpdate = (function() {

        /**
         * Properties of a S2C_InventoryUpdate.
         * @memberof proto
         * @interface IS2C_InventoryUpdate
         * @property {Array.<proto.IInventoryState>|null} [updated] S2C_InventoryUpdate updated
         */

        /**
         * Constructs a new S2C_InventoryUpdate.
         * @memberof proto
         * @classdesc Represents a S2C_InventoryUpdate.
         * @implements IS2C_InventoryUpdate
         * @constructor
         * @param {proto.IS2C_InventoryUpdate=} [properties] Properties to set
         */
        function S2C_InventoryUpdate(properties) {
            this.updated = [];
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * S2C_InventoryUpdate updated.
         * @member {Array.<proto.IInventoryState>} updated
         * @memberof proto.S2C_InventoryUpdate
         * @instance
         */
        S2C_InventoryUpdate.prototype.updated = $util.emptyArray;

        /**
         * Creates a new S2C_InventoryUpdate instance using the specified properties.
         * @function create
         * @memberof proto.S2C_InventoryUpdate
         * @static
         * @param {proto.IS2C_InventoryUpdate=} [properties] Properties to set
         * @returns {proto.S2C_InventoryUpdate} S2C_InventoryUpdate instance
         */
        S2C_InventoryUpdate.create = function create(properties) {
            return new S2C_InventoryUpdate(properties);
        };

        /**
         * Encodes the specified S2C_InventoryUpdate message. Does not implicitly {@link proto.S2C_InventoryUpdate.verify|verify} messages.
         * @function encode
         * @memberof proto.S2C_InventoryUpdate
         * @static
         * @param {proto.IS2C_InventoryUpdate} message S2C_InventoryUpdate message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_InventoryUpdate.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.updated != null && message.updated.length)
                for (let i = 0; i < message.updated.length; ++i)
                    $root.proto.InventoryState.encode(message.updated[i], writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified S2C_InventoryUpdate message, length delimited. Does not implicitly {@link proto.S2C_InventoryUpdate.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.S2C_InventoryUpdate
         * @static
         * @param {proto.IS2C_InventoryUpdate} message S2C_InventoryUpdate message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_InventoryUpdate.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a S2C_InventoryUpdate message from the specified reader or buffer.
         * @function decode
         * @memberof proto.S2C_InventoryUpdate
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.S2C_InventoryUpdate} S2C_InventoryUpdate
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_InventoryUpdate.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.S2C_InventoryUpdate();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        if (!(message.updated && message.updated.length))
                            message.updated = [];
                        message.updated.push($root.proto.InventoryState.decode(reader, reader.uint32()));
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a S2C_InventoryUpdate message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.S2C_InventoryUpdate
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.S2C_InventoryUpdate} S2C_InventoryUpdate
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_InventoryUpdate.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a S2C_InventoryUpdate message.
         * @function verify
         * @memberof proto.S2C_InventoryUpdate
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        S2C_InventoryUpdate.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.updated != null && message.hasOwnProperty("updated")) {
                if (!Array.isArray(message.updated))
                    return "updated: array expected";
                for (let i = 0; i < message.updated.length; ++i) {
                    let error = $root.proto.InventoryState.verify(message.updated[i]);
                    if (error)
                        return "updated." + error;
                }
            }
            return null;
        };

        /**
         * Creates a S2C_InventoryUpdate message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.S2C_InventoryUpdate
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.S2C_InventoryUpdate} S2C_InventoryUpdate
         */
        S2C_InventoryUpdate.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.S2C_InventoryUpdate)
                return object;
            let message = new $root.proto.S2C_InventoryUpdate();
            if (object.updated) {
                if (!Array.isArray(object.updated))
                    throw TypeError(".proto.S2C_InventoryUpdate.updated: array expected");
                message.updated = [];
                for (let i = 0; i < object.updated.length; ++i) {
                    if (typeof object.updated[i] !== "object")
                        throw TypeError(".proto.S2C_InventoryUpdate.updated: object expected");
                    message.updated[i] = $root.proto.InventoryState.fromObject(object.updated[i]);
                }
            }
            return message;
        };

        /**
         * Creates a plain object from a S2C_InventoryUpdate message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.S2C_InventoryUpdate
         * @static
         * @param {proto.S2C_InventoryUpdate} message S2C_InventoryUpdate
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        S2C_InventoryUpdate.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.arrays || options.defaults)
                object.updated = [];
            if (message.updated && message.updated.length) {
                object.updated = [];
                for (let j = 0; j < message.updated.length; ++j)
                    object.updated[j] = $root.proto.InventoryState.toObject(message.updated[j], options);
            }
            return object;
        };

        /**
         * Converts this S2C_InventoryUpdate to JSON.
         * @function toJSON
         * @memberof proto.S2C_InventoryUpdate
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        S2C_InventoryUpdate.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for S2C_InventoryUpdate
         * @function getTypeUrl
         * @memberof proto.S2C_InventoryUpdate
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        S2C_InventoryUpdate.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.S2C_InventoryUpdate";
        };

        return S2C_InventoryUpdate;
    })();

    proto.S2C_ContainerOpened = (function() {

        /**
         * Properties of a S2C_ContainerOpened.
         * @memberof proto
         * @interface IS2C_ContainerOpened
         * @property {proto.IInventoryState|null} [state] S2C_ContainerOpened state
         */

        /**
         * Constructs a new S2C_ContainerOpened.
         * @memberof proto
         * @classdesc Represents a S2C_ContainerOpened.
         * @implements IS2C_ContainerOpened
         * @constructor
         * @param {proto.IS2C_ContainerOpened=} [properties] Properties to set
         */
        function S2C_ContainerOpened(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * S2C_ContainerOpened state.
         * @member {proto.IInventoryState|null|undefined} state
         * @memberof proto.S2C_ContainerOpened
         * @instance
         */
        S2C_ContainerOpened.prototype.state = null;

        /**
         * Creates a new S2C_ContainerOpened instance using the specified properties.
         * @function create
         * @memberof proto.S2C_ContainerOpened
         * @static
         * @param {proto.IS2C_ContainerOpened=} [properties] Properties to set
         * @returns {proto.S2C_ContainerOpened} S2C_ContainerOpened instance
         */
        S2C_ContainerOpened.create = function create(properties) {
            return new S2C_ContainerOpened(properties);
        };

        /**
         * Encodes the specified S2C_ContainerOpened message. Does not implicitly {@link proto.S2C_ContainerOpened.verify|verify} messages.
         * @function encode
         * @memberof proto.S2C_ContainerOpened
         * @static
         * @param {proto.IS2C_ContainerOpened} message S2C_ContainerOpened message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_ContainerOpened.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.state != null && Object.hasOwnProperty.call(message, "state"))
                $root.proto.InventoryState.encode(message.state, writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified S2C_ContainerOpened message, length delimited. Does not implicitly {@link proto.S2C_ContainerOpened.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.S2C_ContainerOpened
         * @static
         * @param {proto.IS2C_ContainerOpened} message S2C_ContainerOpened message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_ContainerOpened.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a S2C_ContainerOpened message from the specified reader or buffer.
         * @function decode
         * @memberof proto.S2C_ContainerOpened
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.S2C_ContainerOpened} S2C_ContainerOpened
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_ContainerOpened.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.S2C_ContainerOpened();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.state = $root.proto.InventoryState.decode(reader, reader.uint32());
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a S2C_ContainerOpened message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.S2C_ContainerOpened
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.S2C_ContainerOpened} S2C_ContainerOpened
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_ContainerOpened.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a S2C_ContainerOpened message.
         * @function verify
         * @memberof proto.S2C_ContainerOpened
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        S2C_ContainerOpened.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.state != null && message.hasOwnProperty("state")) {
                let error = $root.proto.InventoryState.verify(message.state);
                if (error)
                    return "state." + error;
            }
            return null;
        };

        /**
         * Creates a S2C_ContainerOpened message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.S2C_ContainerOpened
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.S2C_ContainerOpened} S2C_ContainerOpened
         */
        S2C_ContainerOpened.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.S2C_ContainerOpened)
                return object;
            let message = new $root.proto.S2C_ContainerOpened();
            if (object.state != null) {
                if (typeof object.state !== "object")
                    throw TypeError(".proto.S2C_ContainerOpened.state: object expected");
                message.state = $root.proto.InventoryState.fromObject(object.state);
            }
            return message;
        };

        /**
         * Creates a plain object from a S2C_ContainerOpened message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.S2C_ContainerOpened
         * @static
         * @param {proto.S2C_ContainerOpened} message S2C_ContainerOpened
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        S2C_ContainerOpened.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults)
                object.state = null;
            if (message.state != null && message.hasOwnProperty("state"))
                object.state = $root.proto.InventoryState.toObject(message.state, options);
            return object;
        };

        /**
         * Converts this S2C_ContainerOpened to JSON.
         * @function toJSON
         * @memberof proto.S2C_ContainerOpened
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        S2C_ContainerOpened.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for S2C_ContainerOpened
         * @function getTypeUrl
         * @memberof proto.S2C_ContainerOpened
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        S2C_ContainerOpened.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.S2C_ContainerOpened";
        };

        return S2C_ContainerOpened;
    })();

    proto.S2C_ContainerClosed = (function() {

        /**
         * Properties of a S2C_ContainerClosed.
         * @memberof proto
         * @interface IS2C_ContainerClosed
         * @property {number|Long|null} [entityId] S2C_ContainerClosed entityId
         */

        /**
         * Constructs a new S2C_ContainerClosed.
         * @memberof proto
         * @classdesc Represents a S2C_ContainerClosed.
         * @implements IS2C_ContainerClosed
         * @constructor
         * @param {proto.IS2C_ContainerClosed=} [properties] Properties to set
         */
        function S2C_ContainerClosed(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * S2C_ContainerClosed entityId.
         * @member {number|Long} entityId
         * @memberof proto.S2C_ContainerClosed
         * @instance
         */
        S2C_ContainerClosed.prototype.entityId = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * Creates a new S2C_ContainerClosed instance using the specified properties.
         * @function create
         * @memberof proto.S2C_ContainerClosed
         * @static
         * @param {proto.IS2C_ContainerClosed=} [properties] Properties to set
         * @returns {proto.S2C_ContainerClosed} S2C_ContainerClosed instance
         */
        S2C_ContainerClosed.create = function create(properties) {
            return new S2C_ContainerClosed(properties);
        };

        /**
         * Encodes the specified S2C_ContainerClosed message. Does not implicitly {@link proto.S2C_ContainerClosed.verify|verify} messages.
         * @function encode
         * @memberof proto.S2C_ContainerClosed
         * @static
         * @param {proto.IS2C_ContainerClosed} message S2C_ContainerClosed message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_ContainerClosed.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.entityId != null && Object.hasOwnProperty.call(message, "entityId"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint64(message.entityId);
            return writer;
        };

        /**
         * Encodes the specified S2C_ContainerClosed message, length delimited. Does not implicitly {@link proto.S2C_ContainerClosed.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.S2C_ContainerClosed
         * @static
         * @param {proto.IS2C_ContainerClosed} message S2C_ContainerClosed message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_ContainerClosed.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a S2C_ContainerClosed message from the specified reader or buffer.
         * @function decode
         * @memberof proto.S2C_ContainerClosed
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.S2C_ContainerClosed} S2C_ContainerClosed
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_ContainerClosed.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.S2C_ContainerClosed();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.entityId = reader.uint64();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a S2C_ContainerClosed message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.S2C_ContainerClosed
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.S2C_ContainerClosed} S2C_ContainerClosed
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_ContainerClosed.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a S2C_ContainerClosed message.
         * @function verify
         * @memberof proto.S2C_ContainerClosed
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        S2C_ContainerClosed.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.entityId != null && message.hasOwnProperty("entityId"))
                if (!$util.isInteger(message.entityId) && !(message.entityId && $util.isInteger(message.entityId.low) && $util.isInteger(message.entityId.high)))
                    return "entityId: integer|Long expected";
            return null;
        };

        /**
         * Creates a S2C_ContainerClosed message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.S2C_ContainerClosed
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.S2C_ContainerClosed} S2C_ContainerClosed
         */
        S2C_ContainerClosed.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.S2C_ContainerClosed)
                return object;
            let message = new $root.proto.S2C_ContainerClosed();
            if (object.entityId != null)
                if ($util.Long)
                    (message.entityId = $util.Long.fromValue(object.entityId)).unsigned = true;
                else if (typeof object.entityId === "string")
                    message.entityId = parseInt(object.entityId, 10);
                else if (typeof object.entityId === "number")
                    message.entityId = object.entityId;
                else if (typeof object.entityId === "object")
                    message.entityId = new $util.LongBits(object.entityId.low >>> 0, object.entityId.high >>> 0).toNumber(true);
            return message;
        };

        /**
         * Creates a plain object from a S2C_ContainerClosed message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.S2C_ContainerClosed
         * @static
         * @param {proto.S2C_ContainerClosed} message S2C_ContainerClosed
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        S2C_ContainerClosed.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults)
                if ($util.Long) {
                    let long = new $util.Long(0, 0, true);
                    object.entityId = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.entityId = options.longs === String ? "0" : 0;
            if (message.entityId != null && message.hasOwnProperty("entityId"))
                if (typeof message.entityId === "number")
                    object.entityId = options.longs === String ? String(message.entityId) : message.entityId;
                else
                    object.entityId = options.longs === String ? $util.Long.prototype.toString.call(message.entityId) : options.longs === Number ? new $util.LongBits(message.entityId.low >>> 0, message.entityId.high >>> 0).toNumber(true) : message.entityId;
            return object;
        };

        /**
         * Converts this S2C_ContainerClosed to JSON.
         * @function toJSON
         * @memberof proto.S2C_ContainerClosed
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        S2C_ContainerClosed.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for S2C_ContainerClosed
         * @function getTypeUrl
         * @memberof proto.S2C_ContainerClosed
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        S2C_ContainerClosed.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.S2C_ContainerClosed";
        };

        return S2C_ContainerClosed;
    })();

    proto.S2C_Error = (function() {

        /**
         * Properties of a S2C_Error.
         * @memberof proto
         * @interface IS2C_Error
         * @property {proto.ErrorCode|null} [code] S2C_Error code
         * @property {string|null} [message] S2C_Error message
         */

        /**
         * Constructs a new S2C_Error.
         * @memberof proto
         * @classdesc Represents a S2C_Error.
         * @implements IS2C_Error
         * @constructor
         * @param {proto.IS2C_Error=} [properties] Properties to set
         */
        function S2C_Error(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * S2C_Error code.
         * @member {proto.ErrorCode} code
         * @memberof proto.S2C_Error
         * @instance
         */
        S2C_Error.prototype.code = 0;

        /**
         * S2C_Error message.
         * @member {string} message
         * @memberof proto.S2C_Error
         * @instance
         */
        S2C_Error.prototype.message = "";

        /**
         * Creates a new S2C_Error instance using the specified properties.
         * @function create
         * @memberof proto.S2C_Error
         * @static
         * @param {proto.IS2C_Error=} [properties] Properties to set
         * @returns {proto.S2C_Error} S2C_Error instance
         */
        S2C_Error.create = function create(properties) {
            return new S2C_Error(properties);
        };

        /**
         * Encodes the specified S2C_Error message. Does not implicitly {@link proto.S2C_Error.verify|verify} messages.
         * @function encode
         * @memberof proto.S2C_Error
         * @static
         * @param {proto.IS2C_Error} message S2C_Error message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_Error.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.code != null && Object.hasOwnProperty.call(message, "code"))
                writer.uint32(/* id 1, wireType 0 =*/8).int32(message.code);
            if (message.message != null && Object.hasOwnProperty.call(message, "message"))
                writer.uint32(/* id 2, wireType 2 =*/18).string(message.message);
            return writer;
        };

        /**
         * Encodes the specified S2C_Error message, length delimited. Does not implicitly {@link proto.S2C_Error.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.S2C_Error
         * @static
         * @param {proto.IS2C_Error} message S2C_Error message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_Error.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a S2C_Error message from the specified reader or buffer.
         * @function decode
         * @memberof proto.S2C_Error
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.S2C_Error} S2C_Error
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_Error.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.S2C_Error();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.code = reader.int32();
                        break;
                    }
                case 2: {
                        message.message = reader.string();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a S2C_Error message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.S2C_Error
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.S2C_Error} S2C_Error
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_Error.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a S2C_Error message.
         * @function verify
         * @memberof proto.S2C_Error
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        S2C_Error.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.code != null && message.hasOwnProperty("code"))
                switch (message.code) {
                default:
                    return "code: enum value expected";
                case 0:
                case 1:
                case 2:
                case 3:
                case 4:
                case 5:
                case 6:
                case 7:
                case 8:
                case 9:
                case 10:
                case 11:
                case 12:
                case 13:
                case 14:
                case 15:
                case 16:
                    break;
                }
            if (message.message != null && message.hasOwnProperty("message"))
                if (!$util.isString(message.message))
                    return "message: string expected";
            return null;
        };

        /**
         * Creates a S2C_Error message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.S2C_Error
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.S2C_Error} S2C_Error
         */
        S2C_Error.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.S2C_Error)
                return object;
            let message = new $root.proto.S2C_Error();
            switch (object.code) {
            default:
                if (typeof object.code === "number") {
                    message.code = object.code;
                    break;
                }
                break;
            case "ERROR_CODE_NONE":
            case 0:
                message.code = 0;
                break;
            case "ERROR_CODE_INVALID_REQUEST":
            case 1:
                message.code = 1;
                break;
            case "ERROR_CODE_NOT_AUTHENTICATED":
            case 2:
                message.code = 2;
                break;
            case "ERROR_CODE_ENTITY_NOT_FOUND":
            case 3:
                message.code = 3;
                break;
            case "ERROR_CODE_OUT_OF_RANGE":
            case 4:
                message.code = 4;
                break;
            case "ERROR_CODE_INSUFFICIENT_RESOURCES":
            case 5:
                message.code = 5;
                break;
            case "ERROR_CODE_INVENTORY_FULL":
            case 6:
                message.code = 6;
                break;
            case "ERROR_CODE_CANNOT_INTERACT":
            case 7:
                message.code = 7;
                break;
            case "ERROR_CODE_COOLDOWN_ACTIVE":
            case 8:
                message.code = 8;
                break;
            case "ERROR_CODE_INSUFFICIENT_STAMINA":
            case 9:
                message.code = 9;
                break;
            case "ERROR_CODE_TARGET_INVALID":
            case 10:
                message.code = 10;
                break;
            case "ERROR_CODE_PATH_BLOCKED":
            case 11:
                message.code = 11;
                break;
            case "ERROR_CODE_TIMEOUT_EXCEEDED":
            case 12:
                message.code = 12;
                break;
            case "ERROR_CODE_BUILDING_INCOMPLETE":
            case 13:
                message.code = 13;
                break;
            case "ERROR_CODE_RECIPE_UNKNOWN":
            case 14:
                message.code = 14;
                break;
            case "ERROR_PACKET_PER_SECOND_LIMIT_THRESHOLDED":
            case 15:
                message.code = 15;
                break;
            case "ERROR_CODE_INTERNAL_ERROR":
            case 16:
                message.code = 16;
                break;
            }
            if (object.message != null)
                message.message = String(object.message);
            return message;
        };

        /**
         * Creates a plain object from a S2C_Error message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.S2C_Error
         * @static
         * @param {proto.S2C_Error} message S2C_Error
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        S2C_Error.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.code = options.enums === String ? "ERROR_CODE_NONE" : 0;
                object.message = "";
            }
            if (message.code != null && message.hasOwnProperty("code"))
                object.code = options.enums === String ? $root.proto.ErrorCode[message.code] === undefined ? message.code : $root.proto.ErrorCode[message.code] : message.code;
            if (message.message != null && message.hasOwnProperty("message"))
                object.message = message.message;
            return object;
        };

        /**
         * Converts this S2C_Error to JSON.
         * @function toJSON
         * @memberof proto.S2C_Error
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        S2C_Error.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for S2C_Error
         * @function getTypeUrl
         * @memberof proto.S2C_Error
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        S2C_Error.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.S2C_Error";
        };

        return S2C_Error;
    })();

    proto.S2C_Warning = (function() {

        /**
         * Properties of a S2C_Warning.
         * @memberof proto
         * @interface IS2C_Warning
         * @property {proto.WarningCode|null} [code] S2C_Warning code
         * @property {string|null} [message] S2C_Warning message
         */

        /**
         * Constructs a new S2C_Warning.
         * @memberof proto
         * @classdesc Represents a S2C_Warning.
         * @implements IS2C_Warning
         * @constructor
         * @param {proto.IS2C_Warning=} [properties] Properties to set
         */
        function S2C_Warning(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * S2C_Warning code.
         * @member {proto.WarningCode} code
         * @memberof proto.S2C_Warning
         * @instance
         */
        S2C_Warning.prototype.code = 0;

        /**
         * S2C_Warning message.
         * @member {string} message
         * @memberof proto.S2C_Warning
         * @instance
         */
        S2C_Warning.prototype.message = "";

        /**
         * Creates a new S2C_Warning instance using the specified properties.
         * @function create
         * @memberof proto.S2C_Warning
         * @static
         * @param {proto.IS2C_Warning=} [properties] Properties to set
         * @returns {proto.S2C_Warning} S2C_Warning instance
         */
        S2C_Warning.create = function create(properties) {
            return new S2C_Warning(properties);
        };

        /**
         * Encodes the specified S2C_Warning message. Does not implicitly {@link proto.S2C_Warning.verify|verify} messages.
         * @function encode
         * @memberof proto.S2C_Warning
         * @static
         * @param {proto.IS2C_Warning} message S2C_Warning message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_Warning.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.code != null && Object.hasOwnProperty.call(message, "code"))
                writer.uint32(/* id 1, wireType 0 =*/8).int32(message.code);
            if (message.message != null && Object.hasOwnProperty.call(message, "message"))
                writer.uint32(/* id 2, wireType 2 =*/18).string(message.message);
            return writer;
        };

        /**
         * Encodes the specified S2C_Warning message, length delimited. Does not implicitly {@link proto.S2C_Warning.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.S2C_Warning
         * @static
         * @param {proto.IS2C_Warning} message S2C_Warning message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_Warning.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a S2C_Warning message from the specified reader or buffer.
         * @function decode
         * @memberof proto.S2C_Warning
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.S2C_Warning} S2C_Warning
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_Warning.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.S2C_Warning();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.code = reader.int32();
                        break;
                    }
                case 2: {
                        message.message = reader.string();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a S2C_Warning message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.S2C_Warning
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.S2C_Warning} S2C_Warning
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_Warning.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a S2C_Warning message.
         * @function verify
         * @memberof proto.S2C_Warning
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        S2C_Warning.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.code != null && message.hasOwnProperty("code"))
                switch (message.code) {
                default:
                    return "code: enum value expected";
                case 0:
                    break;
                }
            if (message.message != null && message.hasOwnProperty("message"))
                if (!$util.isString(message.message))
                    return "message: string expected";
            return null;
        };

        /**
         * Creates a S2C_Warning message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.S2C_Warning
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.S2C_Warning} S2C_Warning
         */
        S2C_Warning.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.S2C_Warning)
                return object;
            let message = new $root.proto.S2C_Warning();
            switch (object.code) {
            default:
                if (typeof object.code === "number") {
                    message.code = object.code;
                    break;
                }
                break;
            case "WARN_INPUT_QUEUE_OVERFLOW":
            case 0:
                message.code = 0;
                break;
            }
            if (object.message != null)
                message.message = String(object.message);
            return message;
        };

        /**
         * Creates a plain object from a S2C_Warning message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.S2C_Warning
         * @static
         * @param {proto.S2C_Warning} message S2C_Warning
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        S2C_Warning.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.code = options.enums === String ? "WARN_INPUT_QUEUE_OVERFLOW" : 0;
                object.message = "";
            }
            if (message.code != null && message.hasOwnProperty("code"))
                object.code = options.enums === String ? $root.proto.WarningCode[message.code] === undefined ? message.code : $root.proto.WarningCode[message.code] : message.code;
            if (message.message != null && message.hasOwnProperty("message"))
                object.message = message.message;
            return object;
        };

        /**
         * Converts this S2C_Warning to JSON.
         * @function toJSON
         * @memberof proto.S2C_Warning
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        S2C_Warning.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for S2C_Warning
         * @function getTypeUrl
         * @memberof proto.S2C_Warning
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        S2C_Warning.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.S2C_Warning";
        };

        return S2C_Warning;
    })();

    proto.ServerMessage = (function() {

        /**
         * Properties of a ServerMessage.
         * @memberof proto
         * @interface IServerMessage
         * @property {number|null} [sequence] ServerMessage sequence
         * @property {proto.IS2C_AuthResult|null} [authResult] ServerMessage authResult
         * @property {proto.IS2C_Pong|null} [pong] ServerMessage pong
         * @property {proto.IS2C_ChunkLoad|null} [chunkLoad] ServerMessage chunkLoad
         * @property {proto.IS2C_ChunkUnload|null} [chunkUnload] ServerMessage chunkUnload
         * @property {proto.IS2C_PlayerEnterWorld|null} [playerEnterWorld] ServerMessage playerEnterWorld
         * @property {proto.IS2C_PlayerLeaveWorld|null} [playerLeaveWorld] ServerMessage playerLeaveWorld
         * @property {proto.IS2C_ObjectSpawn|null} [objectSpawn] ServerMessage objectSpawn
         * @property {proto.IS2C_ObjectDespawn|null} [objectDespawn] ServerMessage objectDespawn
         * @property {proto.IS2C_ObjectMove|null} [objectMove] ServerMessage objectMove
         * @property {proto.IS2C_InventoryOpResult|null} [inventoryOpResult] ServerMessage inventoryOpResult
         * @property {proto.IS2C_InventoryUpdate|null} [inventoryUpdate] ServerMessage inventoryUpdate
         * @property {proto.IS2C_ContainerOpened|null} [containerOpened] ServerMessage containerOpened
         * @property {proto.IS2C_ContainerClosed|null} [containerClosed] ServerMessage containerClosed
         * @property {proto.IS2C_Error|null} [error] ServerMessage error
         * @property {proto.IS2C_Warning|null} [warning] ServerMessage warning
         */

        /**
         * Constructs a new ServerMessage.
         * @memberof proto
         * @classdesc Represents a ServerMessage.
         * @implements IServerMessage
         * @constructor
         * @param {proto.IServerMessage=} [properties] Properties to set
         */
        function ServerMessage(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * ServerMessage sequence.
         * @member {number} sequence
         * @memberof proto.ServerMessage
         * @instance
         */
        ServerMessage.prototype.sequence = 0;

        /**
         * ServerMessage authResult.
         * @member {proto.IS2C_AuthResult|null|undefined} authResult
         * @memberof proto.ServerMessage
         * @instance
         */
        ServerMessage.prototype.authResult = null;

        /**
         * ServerMessage pong.
         * @member {proto.IS2C_Pong|null|undefined} pong
         * @memberof proto.ServerMessage
         * @instance
         */
        ServerMessage.prototype.pong = null;

        /**
         * ServerMessage chunkLoad.
         * @member {proto.IS2C_ChunkLoad|null|undefined} chunkLoad
         * @memberof proto.ServerMessage
         * @instance
         */
        ServerMessage.prototype.chunkLoad = null;

        /**
         * ServerMessage chunkUnload.
         * @member {proto.IS2C_ChunkUnload|null|undefined} chunkUnload
         * @memberof proto.ServerMessage
         * @instance
         */
        ServerMessage.prototype.chunkUnload = null;

        /**
         * ServerMessage playerEnterWorld.
         * @member {proto.IS2C_PlayerEnterWorld|null|undefined} playerEnterWorld
         * @memberof proto.ServerMessage
         * @instance
         */
        ServerMessage.prototype.playerEnterWorld = null;

        /**
         * ServerMessage playerLeaveWorld.
         * @member {proto.IS2C_PlayerLeaveWorld|null|undefined} playerLeaveWorld
         * @memberof proto.ServerMessage
         * @instance
         */
        ServerMessage.prototype.playerLeaveWorld = null;

        /**
         * ServerMessage objectSpawn.
         * @member {proto.IS2C_ObjectSpawn|null|undefined} objectSpawn
         * @memberof proto.ServerMessage
         * @instance
         */
        ServerMessage.prototype.objectSpawn = null;

        /**
         * ServerMessage objectDespawn.
         * @member {proto.IS2C_ObjectDespawn|null|undefined} objectDespawn
         * @memberof proto.ServerMessage
         * @instance
         */
        ServerMessage.prototype.objectDespawn = null;

        /**
         * ServerMessage objectMove.
         * @member {proto.IS2C_ObjectMove|null|undefined} objectMove
         * @memberof proto.ServerMessage
         * @instance
         */
        ServerMessage.prototype.objectMove = null;

        /**
         * ServerMessage inventoryOpResult.
         * @member {proto.IS2C_InventoryOpResult|null|undefined} inventoryOpResult
         * @memberof proto.ServerMessage
         * @instance
         */
        ServerMessage.prototype.inventoryOpResult = null;

        /**
         * ServerMessage inventoryUpdate.
         * @member {proto.IS2C_InventoryUpdate|null|undefined} inventoryUpdate
         * @memberof proto.ServerMessage
         * @instance
         */
        ServerMessage.prototype.inventoryUpdate = null;

        /**
         * ServerMessage containerOpened.
         * @member {proto.IS2C_ContainerOpened|null|undefined} containerOpened
         * @memberof proto.ServerMessage
         * @instance
         */
        ServerMessage.prototype.containerOpened = null;

        /**
         * ServerMessage containerClosed.
         * @member {proto.IS2C_ContainerClosed|null|undefined} containerClosed
         * @memberof proto.ServerMessage
         * @instance
         */
        ServerMessage.prototype.containerClosed = null;

        /**
         * ServerMessage error.
         * @member {proto.IS2C_Error|null|undefined} error
         * @memberof proto.ServerMessage
         * @instance
         */
        ServerMessage.prototype.error = null;

        /**
         * ServerMessage warning.
         * @member {proto.IS2C_Warning|null|undefined} warning
         * @memberof proto.ServerMessage
         * @instance
         */
        ServerMessage.prototype.warning = null;

        // OneOf field names bound to virtual getters and setters
        let $oneOfFields;

        /**
         * ServerMessage payload.
         * @member {"authResult"|"pong"|"chunkLoad"|"chunkUnload"|"playerEnterWorld"|"playerLeaveWorld"|"objectSpawn"|"objectDespawn"|"objectMove"|"inventoryOpResult"|"inventoryUpdate"|"containerOpened"|"containerClosed"|"error"|"warning"|undefined} payload
         * @memberof proto.ServerMessage
         * @instance
         */
        Object.defineProperty(ServerMessage.prototype, "payload", {
            get: $util.oneOfGetter($oneOfFields = ["authResult", "pong", "chunkLoad", "chunkUnload", "playerEnterWorld", "playerLeaveWorld", "objectSpawn", "objectDespawn", "objectMove", "inventoryOpResult", "inventoryUpdate", "containerOpened", "containerClosed", "error", "warning"]),
            set: $util.oneOfSetter($oneOfFields)
        });

        /**
         * Creates a new ServerMessage instance using the specified properties.
         * @function create
         * @memberof proto.ServerMessage
         * @static
         * @param {proto.IServerMessage=} [properties] Properties to set
         * @returns {proto.ServerMessage} ServerMessage instance
         */
        ServerMessage.create = function create(properties) {
            return new ServerMessage(properties);
        };

        /**
         * Encodes the specified ServerMessage message. Does not implicitly {@link proto.ServerMessage.verify|verify} messages.
         * @function encode
         * @memberof proto.ServerMessage
         * @static
         * @param {proto.IServerMessage} message ServerMessage message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        ServerMessage.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.sequence != null && Object.hasOwnProperty.call(message, "sequence"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint32(message.sequence);
            if (message.authResult != null && Object.hasOwnProperty.call(message, "authResult"))
                $root.proto.S2C_AuthResult.encode(message.authResult, writer.uint32(/* id 10, wireType 2 =*/82).fork()).ldelim();
            if (message.pong != null && Object.hasOwnProperty.call(message, "pong"))
                $root.proto.S2C_Pong.encode(message.pong, writer.uint32(/* id 11, wireType 2 =*/90).fork()).ldelim();
            if (message.chunkLoad != null && Object.hasOwnProperty.call(message, "chunkLoad"))
                $root.proto.S2C_ChunkLoad.encode(message.chunkLoad, writer.uint32(/* id 12, wireType 2 =*/98).fork()).ldelim();
            if (message.chunkUnload != null && Object.hasOwnProperty.call(message, "chunkUnload"))
                $root.proto.S2C_ChunkUnload.encode(message.chunkUnload, writer.uint32(/* id 13, wireType 2 =*/106).fork()).ldelim();
            if (message.playerEnterWorld != null && Object.hasOwnProperty.call(message, "playerEnterWorld"))
                $root.proto.S2C_PlayerEnterWorld.encode(message.playerEnterWorld, writer.uint32(/* id 14, wireType 2 =*/114).fork()).ldelim();
            if (message.playerLeaveWorld != null && Object.hasOwnProperty.call(message, "playerLeaveWorld"))
                $root.proto.S2C_PlayerLeaveWorld.encode(message.playerLeaveWorld, writer.uint32(/* id 15, wireType 2 =*/122).fork()).ldelim();
            if (message.objectSpawn != null && Object.hasOwnProperty.call(message, "objectSpawn"))
                $root.proto.S2C_ObjectSpawn.encode(message.objectSpawn, writer.uint32(/* id 16, wireType 2 =*/130).fork()).ldelim();
            if (message.objectDespawn != null && Object.hasOwnProperty.call(message, "objectDespawn"))
                $root.proto.S2C_ObjectDespawn.encode(message.objectDespawn, writer.uint32(/* id 17, wireType 2 =*/138).fork()).ldelim();
            if (message.objectMove != null && Object.hasOwnProperty.call(message, "objectMove"))
                $root.proto.S2C_ObjectMove.encode(message.objectMove, writer.uint32(/* id 18, wireType 2 =*/146).fork()).ldelim();
            if (message.inventoryOpResult != null && Object.hasOwnProperty.call(message, "inventoryOpResult"))
                $root.proto.S2C_InventoryOpResult.encode(message.inventoryOpResult, writer.uint32(/* id 19, wireType 2 =*/154).fork()).ldelim();
            if (message.inventoryUpdate != null && Object.hasOwnProperty.call(message, "inventoryUpdate"))
                $root.proto.S2C_InventoryUpdate.encode(message.inventoryUpdate, writer.uint32(/* id 20, wireType 2 =*/162).fork()).ldelim();
            if (message.containerOpened != null && Object.hasOwnProperty.call(message, "containerOpened"))
                $root.proto.S2C_ContainerOpened.encode(message.containerOpened, writer.uint32(/* id 21, wireType 2 =*/170).fork()).ldelim();
            if (message.containerClosed != null && Object.hasOwnProperty.call(message, "containerClosed"))
                $root.proto.S2C_ContainerClosed.encode(message.containerClosed, writer.uint32(/* id 22, wireType 2 =*/178).fork()).ldelim();
            if (message.error != null && Object.hasOwnProperty.call(message, "error"))
                $root.proto.S2C_Error.encode(message.error, writer.uint32(/* id 42, wireType 2 =*/338).fork()).ldelim();
            if (message.warning != null && Object.hasOwnProperty.call(message, "warning"))
                $root.proto.S2C_Warning.encode(message.warning, writer.uint32(/* id 43, wireType 2 =*/346).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified ServerMessage message, length delimited. Does not implicitly {@link proto.ServerMessage.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.ServerMessage
         * @static
         * @param {proto.IServerMessage} message ServerMessage message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        ServerMessage.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a ServerMessage message from the specified reader or buffer.
         * @function decode
         * @memberof proto.ServerMessage
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.ServerMessage} ServerMessage
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        ServerMessage.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.ServerMessage();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.sequence = reader.uint32();
                        break;
                    }
                case 10: {
                        message.authResult = $root.proto.S2C_AuthResult.decode(reader, reader.uint32());
                        break;
                    }
                case 11: {
                        message.pong = $root.proto.S2C_Pong.decode(reader, reader.uint32());
                        break;
                    }
                case 12: {
                        message.chunkLoad = $root.proto.S2C_ChunkLoad.decode(reader, reader.uint32());
                        break;
                    }
                case 13: {
                        message.chunkUnload = $root.proto.S2C_ChunkUnload.decode(reader, reader.uint32());
                        break;
                    }
                case 14: {
                        message.playerEnterWorld = $root.proto.S2C_PlayerEnterWorld.decode(reader, reader.uint32());
                        break;
                    }
                case 15: {
                        message.playerLeaveWorld = $root.proto.S2C_PlayerLeaveWorld.decode(reader, reader.uint32());
                        break;
                    }
                case 16: {
                        message.objectSpawn = $root.proto.S2C_ObjectSpawn.decode(reader, reader.uint32());
                        break;
                    }
                case 17: {
                        message.objectDespawn = $root.proto.S2C_ObjectDespawn.decode(reader, reader.uint32());
                        break;
                    }
                case 18: {
                        message.objectMove = $root.proto.S2C_ObjectMove.decode(reader, reader.uint32());
                        break;
                    }
                case 19: {
                        message.inventoryOpResult = $root.proto.S2C_InventoryOpResult.decode(reader, reader.uint32());
                        break;
                    }
                case 20: {
                        message.inventoryUpdate = $root.proto.S2C_InventoryUpdate.decode(reader, reader.uint32());
                        break;
                    }
                case 21: {
                        message.containerOpened = $root.proto.S2C_ContainerOpened.decode(reader, reader.uint32());
                        break;
                    }
                case 22: {
                        message.containerClosed = $root.proto.S2C_ContainerClosed.decode(reader, reader.uint32());
                        break;
                    }
                case 42: {
                        message.error = $root.proto.S2C_Error.decode(reader, reader.uint32());
                        break;
                    }
                case 43: {
                        message.warning = $root.proto.S2C_Warning.decode(reader, reader.uint32());
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a ServerMessage message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.ServerMessage
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.ServerMessage} ServerMessage
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        ServerMessage.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a ServerMessage message.
         * @function verify
         * @memberof proto.ServerMessage
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        ServerMessage.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            let properties = {};
            if (message.sequence != null && message.hasOwnProperty("sequence"))
                if (!$util.isInteger(message.sequence))
                    return "sequence: integer expected";
            if (message.authResult != null && message.hasOwnProperty("authResult")) {
                properties.payload = 1;
                {
                    let error = $root.proto.S2C_AuthResult.verify(message.authResult);
                    if (error)
                        return "authResult." + error;
                }
            }
            if (message.pong != null && message.hasOwnProperty("pong")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.proto.S2C_Pong.verify(message.pong);
                    if (error)
                        return "pong." + error;
                }
            }
            if (message.chunkLoad != null && message.hasOwnProperty("chunkLoad")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.proto.S2C_ChunkLoad.verify(message.chunkLoad);
                    if (error)
                        return "chunkLoad." + error;
                }
            }
            if (message.chunkUnload != null && message.hasOwnProperty("chunkUnload")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.proto.S2C_ChunkUnload.verify(message.chunkUnload);
                    if (error)
                        return "chunkUnload." + error;
                }
            }
            if (message.playerEnterWorld != null && message.hasOwnProperty("playerEnterWorld")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.proto.S2C_PlayerEnterWorld.verify(message.playerEnterWorld);
                    if (error)
                        return "playerEnterWorld." + error;
                }
            }
            if (message.playerLeaveWorld != null && message.hasOwnProperty("playerLeaveWorld")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.proto.S2C_PlayerLeaveWorld.verify(message.playerLeaveWorld);
                    if (error)
                        return "playerLeaveWorld." + error;
                }
            }
            if (message.objectSpawn != null && message.hasOwnProperty("objectSpawn")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.proto.S2C_ObjectSpawn.verify(message.objectSpawn);
                    if (error)
                        return "objectSpawn." + error;
                }
            }
            if (message.objectDespawn != null && message.hasOwnProperty("objectDespawn")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.proto.S2C_ObjectDespawn.verify(message.objectDespawn);
                    if (error)
                        return "objectDespawn." + error;
                }
            }
            if (message.objectMove != null && message.hasOwnProperty("objectMove")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.proto.S2C_ObjectMove.verify(message.objectMove);
                    if (error)
                        return "objectMove." + error;
                }
            }
            if (message.inventoryOpResult != null && message.hasOwnProperty("inventoryOpResult")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.proto.S2C_InventoryOpResult.verify(message.inventoryOpResult);
                    if (error)
                        return "inventoryOpResult." + error;
                }
            }
            if (message.inventoryUpdate != null && message.hasOwnProperty("inventoryUpdate")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.proto.S2C_InventoryUpdate.verify(message.inventoryUpdate);
                    if (error)
                        return "inventoryUpdate." + error;
                }
            }
            if (message.containerOpened != null && message.hasOwnProperty("containerOpened")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.proto.S2C_ContainerOpened.verify(message.containerOpened);
                    if (error)
                        return "containerOpened." + error;
                }
            }
            if (message.containerClosed != null && message.hasOwnProperty("containerClosed")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.proto.S2C_ContainerClosed.verify(message.containerClosed);
                    if (error)
                        return "containerClosed." + error;
                }
            }
            if (message.error != null && message.hasOwnProperty("error")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.proto.S2C_Error.verify(message.error);
                    if (error)
                        return "error." + error;
                }
            }
            if (message.warning != null && message.hasOwnProperty("warning")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.proto.S2C_Warning.verify(message.warning);
                    if (error)
                        return "warning." + error;
                }
            }
            return null;
        };

        /**
         * Creates a ServerMessage message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.ServerMessage
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.ServerMessage} ServerMessage
         */
        ServerMessage.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.ServerMessage)
                return object;
            let message = new $root.proto.ServerMessage();
            if (object.sequence != null)
                message.sequence = object.sequence >>> 0;
            if (object.authResult != null) {
                if (typeof object.authResult !== "object")
                    throw TypeError(".proto.ServerMessage.authResult: object expected");
                message.authResult = $root.proto.S2C_AuthResult.fromObject(object.authResult);
            }
            if (object.pong != null) {
                if (typeof object.pong !== "object")
                    throw TypeError(".proto.ServerMessage.pong: object expected");
                message.pong = $root.proto.S2C_Pong.fromObject(object.pong);
            }
            if (object.chunkLoad != null) {
                if (typeof object.chunkLoad !== "object")
                    throw TypeError(".proto.ServerMessage.chunkLoad: object expected");
                message.chunkLoad = $root.proto.S2C_ChunkLoad.fromObject(object.chunkLoad);
            }
            if (object.chunkUnload != null) {
                if (typeof object.chunkUnload !== "object")
                    throw TypeError(".proto.ServerMessage.chunkUnload: object expected");
                message.chunkUnload = $root.proto.S2C_ChunkUnload.fromObject(object.chunkUnload);
            }
            if (object.playerEnterWorld != null) {
                if (typeof object.playerEnterWorld !== "object")
                    throw TypeError(".proto.ServerMessage.playerEnterWorld: object expected");
                message.playerEnterWorld = $root.proto.S2C_PlayerEnterWorld.fromObject(object.playerEnterWorld);
            }
            if (object.playerLeaveWorld != null) {
                if (typeof object.playerLeaveWorld !== "object")
                    throw TypeError(".proto.ServerMessage.playerLeaveWorld: object expected");
                message.playerLeaveWorld = $root.proto.S2C_PlayerLeaveWorld.fromObject(object.playerLeaveWorld);
            }
            if (object.objectSpawn != null) {
                if (typeof object.objectSpawn !== "object")
                    throw TypeError(".proto.ServerMessage.objectSpawn: object expected");
                message.objectSpawn = $root.proto.S2C_ObjectSpawn.fromObject(object.objectSpawn);
            }
            if (object.objectDespawn != null) {
                if (typeof object.objectDespawn !== "object")
                    throw TypeError(".proto.ServerMessage.objectDespawn: object expected");
                message.objectDespawn = $root.proto.S2C_ObjectDespawn.fromObject(object.objectDespawn);
            }
            if (object.objectMove != null) {
                if (typeof object.objectMove !== "object")
                    throw TypeError(".proto.ServerMessage.objectMove: object expected");
                message.objectMove = $root.proto.S2C_ObjectMove.fromObject(object.objectMove);
            }
            if (object.inventoryOpResult != null) {
                if (typeof object.inventoryOpResult !== "object")
                    throw TypeError(".proto.ServerMessage.inventoryOpResult: object expected");
                message.inventoryOpResult = $root.proto.S2C_InventoryOpResult.fromObject(object.inventoryOpResult);
            }
            if (object.inventoryUpdate != null) {
                if (typeof object.inventoryUpdate !== "object")
                    throw TypeError(".proto.ServerMessage.inventoryUpdate: object expected");
                message.inventoryUpdate = $root.proto.S2C_InventoryUpdate.fromObject(object.inventoryUpdate);
            }
            if (object.containerOpened != null) {
                if (typeof object.containerOpened !== "object")
                    throw TypeError(".proto.ServerMessage.containerOpened: object expected");
                message.containerOpened = $root.proto.S2C_ContainerOpened.fromObject(object.containerOpened);
            }
            if (object.containerClosed != null) {
                if (typeof object.containerClosed !== "object")
                    throw TypeError(".proto.ServerMessage.containerClosed: object expected");
                message.containerClosed = $root.proto.S2C_ContainerClosed.fromObject(object.containerClosed);
            }
            if (object.error != null) {
                if (typeof object.error !== "object")
                    throw TypeError(".proto.ServerMessage.error: object expected");
                message.error = $root.proto.S2C_Error.fromObject(object.error);
            }
            if (object.warning != null) {
                if (typeof object.warning !== "object")
                    throw TypeError(".proto.ServerMessage.warning: object expected");
                message.warning = $root.proto.S2C_Warning.fromObject(object.warning);
            }
            return message;
        };

        /**
         * Creates a plain object from a ServerMessage message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.ServerMessage
         * @static
         * @param {proto.ServerMessage} message ServerMessage
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        ServerMessage.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults)
                object.sequence = 0;
            if (message.sequence != null && message.hasOwnProperty("sequence"))
                object.sequence = message.sequence;
            if (message.authResult != null && message.hasOwnProperty("authResult")) {
                object.authResult = $root.proto.S2C_AuthResult.toObject(message.authResult, options);
                if (options.oneofs)
                    object.payload = "authResult";
            }
            if (message.pong != null && message.hasOwnProperty("pong")) {
                object.pong = $root.proto.S2C_Pong.toObject(message.pong, options);
                if (options.oneofs)
                    object.payload = "pong";
            }
            if (message.chunkLoad != null && message.hasOwnProperty("chunkLoad")) {
                object.chunkLoad = $root.proto.S2C_ChunkLoad.toObject(message.chunkLoad, options);
                if (options.oneofs)
                    object.payload = "chunkLoad";
            }
            if (message.chunkUnload != null && message.hasOwnProperty("chunkUnload")) {
                object.chunkUnload = $root.proto.S2C_ChunkUnload.toObject(message.chunkUnload, options);
                if (options.oneofs)
                    object.payload = "chunkUnload";
            }
            if (message.playerEnterWorld != null && message.hasOwnProperty("playerEnterWorld")) {
                object.playerEnterWorld = $root.proto.S2C_PlayerEnterWorld.toObject(message.playerEnterWorld, options);
                if (options.oneofs)
                    object.payload = "playerEnterWorld";
            }
            if (message.playerLeaveWorld != null && message.hasOwnProperty("playerLeaveWorld")) {
                object.playerLeaveWorld = $root.proto.S2C_PlayerLeaveWorld.toObject(message.playerLeaveWorld, options);
                if (options.oneofs)
                    object.payload = "playerLeaveWorld";
            }
            if (message.objectSpawn != null && message.hasOwnProperty("objectSpawn")) {
                object.objectSpawn = $root.proto.S2C_ObjectSpawn.toObject(message.objectSpawn, options);
                if (options.oneofs)
                    object.payload = "objectSpawn";
            }
            if (message.objectDespawn != null && message.hasOwnProperty("objectDespawn")) {
                object.objectDespawn = $root.proto.S2C_ObjectDespawn.toObject(message.objectDespawn, options);
                if (options.oneofs)
                    object.payload = "objectDespawn";
            }
            if (message.objectMove != null && message.hasOwnProperty("objectMove")) {
                object.objectMove = $root.proto.S2C_ObjectMove.toObject(message.objectMove, options);
                if (options.oneofs)
                    object.payload = "objectMove";
            }
            if (message.inventoryOpResult != null && message.hasOwnProperty("inventoryOpResult")) {
                object.inventoryOpResult = $root.proto.S2C_InventoryOpResult.toObject(message.inventoryOpResult, options);
                if (options.oneofs)
                    object.payload = "inventoryOpResult";
            }
            if (message.inventoryUpdate != null && message.hasOwnProperty("inventoryUpdate")) {
                object.inventoryUpdate = $root.proto.S2C_InventoryUpdate.toObject(message.inventoryUpdate, options);
                if (options.oneofs)
                    object.payload = "inventoryUpdate";
            }
            if (message.containerOpened != null && message.hasOwnProperty("containerOpened")) {
                object.containerOpened = $root.proto.S2C_ContainerOpened.toObject(message.containerOpened, options);
                if (options.oneofs)
                    object.payload = "containerOpened";
            }
            if (message.containerClosed != null && message.hasOwnProperty("containerClosed")) {
                object.containerClosed = $root.proto.S2C_ContainerClosed.toObject(message.containerClosed, options);
                if (options.oneofs)
                    object.payload = "containerClosed";
            }
            if (message.error != null && message.hasOwnProperty("error")) {
                object.error = $root.proto.S2C_Error.toObject(message.error, options);
                if (options.oneofs)
                    object.payload = "error";
            }
            if (message.warning != null && message.hasOwnProperty("warning")) {
                object.warning = $root.proto.S2C_Warning.toObject(message.warning, options);
                if (options.oneofs)
                    object.payload = "warning";
            }
            return object;
        };

        /**
         * Converts this ServerMessage to JSON.
         * @function toJSON
         * @memberof proto.ServerMessage
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        ServerMessage.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for ServerMessage
         * @function getTypeUrl
         * @memberof proto.ServerMessage
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        ServerMessage.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.ServerMessage";
        };

        return ServerMessage;
    })();

    return proto;
})();

export { $root as default };
